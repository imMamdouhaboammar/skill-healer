package healer

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/imMamdouhaboammar/skill-healer/pkg/telemetry"
	"github.com/imMamdouhaboammar/skill-healer/pkg/types"
)

// DedupeResult reports the outcome of a deduplication run.
type DedupeResult struct {
	TotalScanned     int      `json:"total_scanned"`
	AlreadyLinks     int      `json:"already_links"`
	Converted        int      `json:"converted"`
	StaleRemoved     int      `json:"stale_removed"`
	SkippedDifferent int      `json:"skipped_different"`
	SkippedUnsafe    int      `json:"skipped_unsafe"`
	Backups          []string `json:"backups,omitempty"`
	Errors           []string `json:"errors,omitempty"`
}

// Dedupe examines the legacy Gemini mirror. Same-named skills are only linked
// when their complete regular-file trees match; originals are never deleted.
func Dedupe(dryRun bool) (*DedupeResult, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	return DedupeAt(filepath.Join(home, ".gemini", "config", "skills"), filepath.Join(home, ".agents", "skills"), filepath.Join(home, ".ash", "vault", "dedupe"), dryRun)
}

// DedupeAt allows isolated filesystem tests and preserves a recovery copy for
// each successful conversion. No recovery directory is removed automatically.
func DedupeAt(geminiSkills, canonicalSkills, vault string, dryRun bool) (*DedupeResult, error) {
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
		// Existing backups belong to the user. Never auto-delete them.
		if strings.HasSuffix(name, ".stale-backup") || strings.HasSuffix(name, ".bak") {
			res.SkippedUnsafe++
			continue
		}
		if lInfo.Mode()&os.ModeSymlink != 0 {
			res.AlreadyLinks++
			continue
		}
		canonicalInfo, statErr := os.Lstat(cPath)
		if os.IsNotExist(statErr) {
			continue
		}
		if statErr != nil || !canonicalInfo.IsDir() || !lInfo.IsDir() {
			res.SkippedUnsafe++
			continue
		}
		originalHash, firstErr := skillTreeHash(gPath)
		canonicalHash, secondErr := skillTreeHash(cPath)
		if firstErr != nil || secondErr != nil {
			res.SkippedUnsafe++
			continue
		}
		if originalHash != canonicalHash {
			res.SkippedDifferent++
			continue
		}
		if dryRun {
			res.Converted++
			continue
		}
		if err := os.MkdirAll(vault, 0700); err != nil {
			res.Errors = append(res.Errors, err.Error())
			continue
		}
		slot, err := os.MkdirTemp(vault, "copy-*")
		if err != nil {
			res.Errors = append(res.Errors, err.Error())
			continue
		}
		backup := filepath.Join(slot, name)
		// Recheck immediately before moving, to avoid converting a changed tree.
		checkHash, checkErr := skillTreeHash(gPath)
		canonicalCheck, canonicalErr := skillTreeHash(cPath)
		if checkErr != nil || checkHash != originalHash || canonicalErr != nil || canonicalCheck != canonicalHash {
			res.Errors = append(res.Errors, fmt.Sprintf("source or canonical content changed during review: %s", gPath))
			continue
		}
		if err := os.Rename(gPath, backup); err != nil {
			res.Errors = append(res.Errors, fmt.Sprintf("cannot archive %s: %v", gPath, err))
			continue
		}
		if err := os.Symlink(cPath, gPath); err != nil {
			if restoreErr := os.Rename(backup, gPath); restoreErr != nil {
				res.Errors = append(res.Errors, fmt.Sprintf("cannot create link: %v; recovery copy retained at %s, automatic restore failed: %v", err, backup, restoreErr))
			} else {
				res.Errors = append(res.Errors, fmt.Sprintf("cannot create link, original restored: %v", err))
			}
			continue
		}
		res.Backups = append(res.Backups, backup)
		res.Converted++
	}
	return res, nil
}

// skillTreeHash compares complete regular-file trees, including relative paths
// and directory structure. Symlinks and special files are rejected rather than
// followed. A real SKILL.md is mandatory on both sides.
func skillTreeHash(root string) (string, error) {
	manifest, err := os.Lstat(filepath.Join(root, "SKILL.md"))
	if err != nil || !manifest.Mode().IsRegular() {
		return "", fmt.Errorf("missing regular SKILL.md in %s", root)
	}
	var paths []string
	err = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 || (!entry.IsDir() && !entry.Type().IsRegular()) {
			return fmt.Errorf("unsafe entry %s", path)
		}
		paths = append(paths, path)
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Strings(paths)
	h := sha256.New()
	for _, path := range paths {
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return "", err
		}
		info, err := os.Lstat(path)
		if err != nil {
			return "", err
		}
		if info.IsDir() {
			_, _ = io.WriteString(h, "D\x00"+rel+"\x00")
			continue
		}
		if !info.Mode().IsRegular() {
			return "", fmt.Errorf("unsafe file %s", path)
		}
		_, _ = io.WriteString(h, "F\x00"+rel+"\x00")
		file, err := os.Open(path)
		if err != nil {
			return "", err
		}
		_, copyErr := io.Copy(h, file)
		closeErr := file.Close()
		if copyErr != nil {
			return "", copyErr
		}
		if closeErr != nil {
			return "", closeErr
		}
		_, _ = io.WriteString(h, "\x00")
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// QuietResult reports on quiet mode activation.
type QuietResult struct {
	TotalBefore  int      `json:"total_before"`
	KeptActive   int      `json:"kept_active"`
	MovedStandby int      `json:"moved_standby"`
	Skipped      int      `json:"skipped"`
	Errors       []string `json:"errors,omitempty"`
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
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	geminiSkills := filepath.Join(home, ".gemini", "config", "skills")
	standbyDir := filepath.Join(home, ".ash", "vault", "standby")

	if !dryRun {
		if err := os.MkdirAll(standbyDir, 0700); err != nil {
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
			continue
		}
		// Only move links we can restore exactly. Never overwrite a standby
		// entry or touch a real directory whose content may be unique.
		activeInfo, statErr := os.Lstat(gPath)
		if statErr != nil || activeInfo.Mode()&os.ModeSymlink == 0 {
			res.Skipped++
			continue
		}
		targetStandby := filepath.Join(standbyDir, name)
		if _, existingErr := os.Lstat(targetStandby); !os.IsNotExist(existingErr) {
			res.Skipped++
			continue
		}
		if dryRun {
			res.MovedStandby++
			continue
		}
		if err := os.Rename(gPath, targetStandby); err != nil {
			res.Skipped++
			res.Errors = append(res.Errors, fmt.Sprintf("cannot move %s to standby: %v", gPath, err))
			continue
		}
		res.MovedStandby++
	}

	return res, nil
}

// Load dynamically activates a skill from ~/.agents/skills or standby into ~/.gemini/config/skills.
func Load(skillName string) error {
	if err := validateSkillName(skillName); err != nil {
		return err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
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

	activeLink := filepath.Join(geminiSkills, realName)
	if info, err := os.Lstat(activeLink); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			target, readErr := os.Readlink(activeLink)
			if readErr == nil && target == canonical {
				return nil
			}
		}
		return fmt.Errorf("active destination already exists: %s", activeLink)
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(geminiSkills, 0700); err != nil {
		return err
	}
	if err := os.Symlink(canonical, activeLink); err != nil {
		return fmt.Errorf("failed to mount skill: %w", err)
	}
	// A matching dormant alias can now be removed safely; keep all other
	// standby data intact, including real directories and distinct links.
	for _, candidate := range []string{realName, skillName} {
		standby := filepath.Join(standbyDir, candidate)
		if info, err := os.Lstat(standby); err == nil && info.Mode()&os.ModeSymlink != 0 {
			target, readErr := os.Readlink(standby)
			if readErr == nil && target == canonical {
				_ = os.Remove(standby)
			}
		}
	}

	return nil
}

// Unload removes a skill from active set back to standby.
func Unload(skillName string) error {
	if err := validateSkillName(skillName); err != nil {
		return err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	geminiSkills := filepath.Join(home, ".gemini", "config", "skills")
	standbyDir := filepath.Join(home, ".ash", "vault", "standby")
	activeLink := filepath.Join(geminiSkills, skillName)

	info, err := os.Lstat(activeLink)
	if err != nil {
		return fmt.Errorf("skill '%s' is not active", skillName)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return fmt.Errorf("refusing to move physical active directory: %s", activeLink)
	}

	if err := os.MkdirAll(standbyDir, 0700); err != nil {
		return err
	}
	standbyLink := filepath.Join(standbyDir, skillName)
	if _, err := os.Lstat(standbyLink); !os.IsNotExist(err) {
		return fmt.Errorf("standby destination already exists or is inaccessible: %s", standbyLink)
	}

	return os.Rename(activeLink, standbyLink)
}

func validateSkillName(name string) error {
	if name == "" || name == "." || name == ".." || len(name) > 240 || strings.HasPrefix(name, ".") || filepath.Base(name) != name || strings.ContainsAny(name, "/\\\x00\r\n") {
		return fmt.Errorf("invalid skill name")
	}
	return nil
}
