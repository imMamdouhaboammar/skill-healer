package healer

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/imMamdouhaboammar/skill-healer/pkg/telemetry"
	"github.com/imMamdouhaboammar/skill-healer/pkg/types"
)

// DedupeResult reports the outcome of a deduplication run.
type DedupeResult struct {
	TotalScanned int      `json:"total_scanned"`
	AlreadyLinks int      `json:"already_links"`
	Converted    int      `json:"converted"`
	StaleRemoved int      `json:"stale_removed"`
	Errors       []string `json:"errors,omitempty"`
}

// Dedupe converts duplicate physical skill directories in ~/.gemini/config/skills into canonical symlinks.
func Dedupe(dryRun bool) (*DedupeResult, error) {
	home, _ := os.UserHomeDir()
	geminiSkills := filepath.Join(home, ".gemini", "config", "skills")
	canonicalSkills := filepath.Join(home, ".agents", "skills")

	res := &DedupeResult{}

	entries, err := os.ReadDir(geminiSkills)
	if err != nil {
		return nil, fmt.Errorf("reading gemini skills: %w", err)
	}

	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		res.TotalScanned++
		gPath := filepath.Join(geminiSkills, name)
		cPath := filepath.Join(canonicalSkills, name)

		lInfo, lErr := os.Lstat(gPath)
		if lErr != nil {
			continue
		}

		// Handle stale backup folders
		if strings.HasSuffix(name, ".stale-backup") || strings.HasSuffix(name, ".bak") {
			if !dryRun {
				_ = os.RemoveAll(gPath)
			}
			res.StaleRemoved++
			continue
		}

		// Check if already a symlink
		if lInfo.Mode()&os.ModeSymlink != 0 {
			res.AlreadyLinks++
			continue
		}

		// It's a real directory. Check if canonical target exists
		if _, statErr := os.Stat(cPath); statErr == nil {
			// Canonical exists! Replace real dir with symlink
			if !dryRun {
				// Remove directory
				if rmErr := os.RemoveAll(gPath); rmErr != nil {
					res.Errors = append(res.Errors, fmt.Sprintf("failed to remove %s: %v", gPath, rmErr))
					continue
				}
				// Create atomic symlink pointing to canonical
				if symErr := os.Symlink(cPath, gPath); symErr != nil {
					res.Errors = append(res.Errors, fmt.Sprintf("failed to symlink %s -> %s: %v", gPath, cPath, symErr))
					continue
				}
			}
			res.Converted++
		}
	}

	return res, nil
}

// QuietResult reports on quiet mode activation.
type QuietResult struct {
	TotalBefore int `json:"total_before"`
	KeptActive  int `json:"kept_active"`
	MovedStandby int `json:"moved_standby"`
}

// Essential core skills that must always remain in Active Set.
var essentialSkills = map[string]bool{
	"no-mistakes": true, "test-guard": true, "clean-code-guard": true, "fable-watchdog": true,
	"get-fable": true, "fable-plan": true, "fable-verify": true, "fable-architecture": true,
	"fable-tdd": true, "fable-execute": true, "fable-recover": true, "fable-review": true,
	"fable-spark": true, "fable-security": true, "fable-memory": true, "fable-learning": true,
	"fable-cowork": true, "fable-delegate": true, "fable-tend": true, "fable-wise": true,
	"gsd-fast": true, "gsd-progress": true, "gsd-quick": true, "gsd-plan-phase": true,
	"gsd-execute-phase": true, "gsd-verify-work": true, "gsd-code-review": true,
	"writing-plans": true, "executing-plans": true, "deep-digger": true, "j-space": true,
	"check": true, "openreview": true, "superpowers": true, "using-superpowers": true,
	"explore-codebase": true, "debug-issue": true, "refactor-safely": true,
}

// Quiet optimizes ~/.gemini/config/skills by leaving only Hot/Essential skills active.
func Quiet(dryRun bool, minCalls int, db *types.TelemetryDB) (*QuietResult, error) {
	fmt.Printf("DEBUG: Inside healer.Quiet: dryRun=%v, minCalls=%d\n", dryRun, minCalls)
	home, _ := os.UserHomeDir()
	geminiSkills := filepath.Join(home, ".gemini", "config", "skills")
	standbyDir := filepath.Join(home, ".ash", "vault", "standby")

	if !dryRun {
		if err := os.MkdirAll(standbyDir, 0755); err != nil {
			return nil, fmt.Errorf("creating standby dir: %w", err)
		}
	}

	entries, err := os.ReadDir(geminiSkills)
	if err != nil {
		return nil, fmt.Errorf("reading gemini skills: %w", err)
	}

	res := &QuietResult{
		TotalBefore: len(entries),
	}

	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		gPath := filepath.Join(geminiSkills, name)

		isHot := false
		if essentialSkills[name] {
			isHot = true
		} else if db != nil {
			if rec, ok := telemetry.GetRecord(db, types.TypeSkill, name); ok {
				if rec.CallCount >= int64(minCalls) {
					isHot = true
				}
			}
		}

		if isHot {
			res.KeptActive++
		} else {
			res.MovedStandby++
			if !dryRun {
				targetStandby := filepath.Join(standbyDir, name)
				_ = os.RemoveAll(targetStandby)

				linkTarget, readErr := os.Readlink(gPath)
				if readErr == nil {
					_ = os.Symlink(linkTarget, targetStandby)
					rmErr := os.Remove(gPath)
					if rmErr != nil {
						fmt.Printf("Error removing %s: %v\n", gPath, rmErr)
					} else {
						// Check immediately if it was removed
						if _, checkErr := os.Lstat(gPath); checkErr == nil {
							fmt.Printf("CRITICAL ANOMALY: %s still exists immediately after successful os.Remove!\n", gPath)
						}
					}
				} else {
					fmt.Printf("readErr for %s: %v\n", gPath, readErr)
				}
			}
		}
	}

	return res, nil
}

// Load dynamically activates a skill from ~/.agents/skills or standby into ~/.gemini/config/skills.
func Load(skillName string) error {
	home, _ := os.UserHomeDir()
	geminiSkills := filepath.Join(home, ".gemini", "config", "skills")
	canonicalDir := filepath.Join(home, ".agents", "skills")
	standbyDir := filepath.Join(home, ".ash", "vault", "standby")

	// Try direct name, then underscore/hyphen variants
	realName := skillName
	canonical := filepath.Join(canonicalDir, realName)

	if _, err := os.Stat(canonical); err != nil {
		variant := strings.ReplaceAll(skillName, "-", "_")
		if _, err := os.Stat(filepath.Join(canonicalDir, variant)); err == nil {
			realName = variant
			canonical = filepath.Join(canonicalDir, realName)
		} else {
			variant = strings.ReplaceAll(skillName, "_", "-")
			if _, err := os.Stat(filepath.Join(canonicalDir, variant)); err == nil {
				realName = variant
				canonical = filepath.Join(canonicalDir, realName)
			} else {
				return fmt.Errorf("skill '%s' not found in canonical store (%s)", skillName, canonicalDir)
			}
		}
	}

	// Remove from standby vault if parked there
	_ = os.Remove(filepath.Join(standbyDir, realName))
	_ = os.Remove(filepath.Join(standbyDir, skillName))

	activeLink := filepath.Join(geminiSkills, realName)
	_ = os.Remove(activeLink)

	// Create symlink
	if err := os.Symlink(canonical, activeLink); err != nil {
		return fmt.Errorf("failed to mount skill: %w", err)
	}

	return nil
}

// Unload removes a skill from active set back to standby.
func Unload(skillName string) error {
	home, _ := os.UserHomeDir()
	geminiSkills := filepath.Join(home, ".gemini", "config", "skills")
	standbyDir := filepath.Join(home, ".ash", "vault", "standby")
	activeLink := filepath.Join(geminiSkills, skillName)

	if _, err := os.Lstat(activeLink); err != nil {
		return fmt.Errorf("skill '%s' is not active", skillName)
	}

	_ = os.MkdirAll(standbyDir, 0755)
	standbyLink := filepath.Join(standbyDir, skillName)
	_ = os.Remove(standbyLink)

	return os.Rename(activeLink, standbyLink)
}
