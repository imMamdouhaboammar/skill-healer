package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/imMamdouhaboammar/skill-healer/pkg/healer"
	"github.com/imMamdouhaboammar/skill-healer/pkg/instrument"
	"github.com/imMamdouhaboammar/skill-healer/pkg/scanner"
	"github.com/imMamdouhaboammar/skill-healer/pkg/telemetry"
	"github.com/imMamdouhaboammar/skill-healer/pkg/types"
)

const (
	Version = "1.0.0"
	Banner  = `
   _____   _____ _    _ 
  / _ \ \ / / __| |  | |
 | |_| \ V /\__ \ |__| |
  \___| |_| |___/____|_| - AGentic Skills Healer
  Native macOS Pool Management & Telemetry Core
`
)

func main() {
	if len(os.Args) < 2 {
		printHelp()
		os.Exit(0)
	}

	cmd := os.Args[1]
	args := os.Args[2:]

	switch cmd {
	case "dashboard":
		runDashboard(args)
	case "cleanup":
		if err := cleanupCommand(args, os.Stdout); err != nil {
			fmt.Fprintf(os.Stderr, "Cleanup error: %v\n", err)
			os.Exit(1)
		}
	case "quick":
		runQuick(args)
	case "pool":
		runPool(args)
	case "stats":
		runStats(args)
	case "top":
		runTop(args)
	case "dormant":
		runDormant(args)
	case "track":
		runTrack(args)
	case "hook":
		runHook(args)
	case "instrument":
		runInstrument(args)
	case "dedupe":
		runDedupe(args)
	case "quiet":
		runQuiet(args)
	case "load":
		runLoad(args)
	case "unload":
		runUnload(args)
	case "doctor":
		runDoctor(args)
	case "version", "-v", "--version":
		fmt.Printf("AGentic Skills Healer (ash) v%s (darwin/arm64)\n", Version)
	case "help", "-h", "--help":
		printHelp()
	default:
		fmt.Printf("Unknown command: %s\n\n", cmd)
		printHelp()
		os.Exit(1)
	}
}

func printHelp() {
	fmt.Print(Banner)
	fmt.Println(`Usage:
  ash <command> [options] [arguments]

Management & Discovery:
  dashboard       Start the local management dashboard (--port 4317)
  cleanup         Plan, remove, restore or permanently delete eligible tools
  quick           Show pinned tools, commands and collections (--json for agents)
  pool            Display full inventory of CLIs, MCPs, Skills, and Agents
  stats           Display Lifetime Lite Counter statistics and telemetry
  top             Display top most-invoked entities across all categories
  dormant         Display all dormant (unused) skills and MCPs
  doctor          Run health check on symlinks, paths, and configurations

Healing & Noise Control:
  dedupe          Preview identical physical skill copies; --apply archives originals and links them
  quiet           Preview optional standby moves; --apply moves eligible symbolic links
  load <skill>    Dynamically mount any skill into Active Set in 1ms
  unload <skill>  Unmount a skill from Active Set back to Standby

Telemetry:
  track <t> <n>   Record an invocation (t = cli|mcp|skill|agent, n = name)
  hook claude     Receive observed events from Claude Code via stdin
  instrument      Set up host tracking (claude install|status, cli install|status)
  version         Show ash version and architecture`)
}

func runPool(args []string) {
	fs := flag.NewFlagSet("pool", flag.ExitOnError)
	jsonOutput := fs.Bool("json", false, "Output in JSON format")
	filterType := fs.String("type", "", "Filter by type (cli, mcp, skill, agent)")
	_ = fs.Parse(args)

	db, _ := telemetry.Load("")
	pool, err := scanner.ScanPool(db)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error scanning pool: %v\n", err)
		os.Exit(1)
	}

	if *jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(pool)
		return
	}

	fmt.Printf("=== AGentic Skills Healer: Full Pool Inventory ===\n")
	fmt.Printf("Total Tracked Entities: %d (Scanned in: %s)\n\n", pool.Total, time.Since(pool.ScannedAt).Round(time.Millisecond))

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintf(w, "TYPE\tTOTAL\tHOT\tWARM\tDORMANT\n")
	fmt.Fprintf(w, "----\t-----\t---\t----\t-------\n")

	summarizeType := func(tname string, entities map[string]types.Entity) {
		hot, warm, dormant := 0, 0, 0
		for _, e := range entities {
			switch e.Tier {
			case types.TierHot:
				hot++
			case types.TierWarm:
				warm++
			default:
				dormant++
			}
		}
		fmt.Fprintf(w, "%s\t%d\t%d\t%d\t%d\n", tname, len(entities), hot, warm, dormant)
	}

	if *filterType == "" || *filterType == "skill" {
		summarizeType("Skills", pool.Skills)
	}
	if *filterType == "" || *filterType == "mcp" {
		summarizeType("MCP Servers", pool.MCPs)
	}
	if *filterType == "" || *filterType == "cli" {
		summarizeType("CLIs", pool.CLIs)
	}
	if *filterType == "" || *filterType == "agent" {
		summarizeType("Agents", pool.Agents)
	}
	_ = w.Flush()
}

func runStats(args []string) {
	db, err := telemetry.Load("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading telemetry: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("=== Lifetime Lite Counter: Telemetry Stats ===\n")
	fmt.Printf("Database Path:  %s\n", telemetry.DefaultDBPath())
	fmt.Printf("Tracked Records: %d\n", db.TotalEntities)
	fmt.Printf("Last Recorded:   %s\n\n", time.Unix(int64(db.LastUpdated), 0).Format(time.RFC1123))

	typeCounts := make(map[string]int)
	tierCounts := make(map[string]int)
	var totalCalls, observedCalls int64

	for _, rec := range db.Entities {
		typeCounts[rec.Type]++
		tierCounts[rec.Tier]++
		totalCalls += rec.CallCount
		observedCalls += rec.VerifiedCalls
	}

	fmt.Printf("Imported + reported calls: %d (historical provenance not audited)\n", totalCalls)
	fmt.Printf("Host-observed calls:       %d (instrumented events only)\n\n", observedCalls)
	fmt.Println("Breakdown by Activity Tier:")
	for tier, count := range tierCounts {
		fmt.Printf("  • %-8s: %d entities\n", tier, count)
	}
}

func runTop(args []string) {
	db, err := telemetry.Load("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading telemetry: %v\n", err)
		os.Exit(1)
	}

	records := make([]types.TelemetryRecord, 0, len(db.Entities))
	for _, r := range db.Entities {
		records = append(records, r)
	}

	sort.Slice(records, func(i, j int) bool {
		return records[i].CallCount > records[j].CallCount
	})

	fmt.Printf("=== Top 15 Most Invoked Entities (Lifetime) ===\n")
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintf(w, "#\tTYPE\tNAME\tCALLS\tTIER\tLAST USED\n")
	fmt.Fprintf(w, "-\t----\t----\t-----\t----\t---------\n")

	limit := 15
	if len(records) < limit {
		limit = len(records)
	}

	for i := 0; i < limit; i++ {
		r := records[i]
		cleanName := strings.TrimPrefix(r.Name, r.Type+":")
		fmt.Fprintf(w, "%d\t%s\t%s\t%d\t%s\t%.1f days ago\n", i+1, r.Type, cleanName, r.CallCount, r.Tier, r.DaysSinceLast)
	}
	_ = w.Flush()
}

func runDormant(args []string) {
	db, err := telemetry.Load("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading telemetry: %v\n", err)
		os.Exit(1)
	}

	pool, err := scanner.ScanPool(db)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error scanning pool: %v\n", err)
		os.Exit(1)
	}

	dormantSkills := make([]string, 0)
	for name, s := range pool.Skills {
		if s.Tier == types.TierDormant {
			dormantSkills = append(dormantSkills, name)
		}
	}
	sort.Strings(dormantSkills)

	fmt.Printf("=== Dormant Skills (Zero Lifetime Calls) ===\n")
	fmt.Printf("Total Dormant Skills: %d of %d (%.1f%% of skills are candidates for Standby)\n",
		len(dormantSkills), len(pool.Skills), float64(len(dormantSkills))/float64(len(pool.Skills))*100)
	fmt.Printf("Sample of 15 Dormant Skills:\n")
	for i := 0; i < 15 && i < len(dormantSkills); i++ {
		fmt.Printf("  • %s\n", dormantSkills[i])
	}
}

func runTrack(args []string) {
	if len(args) < 2 {
		fmt.Println("Usage: ash track <cli|mcp|skill|agent> <name>")
		os.Exit(1)
	}
	etype := types.EntityType(args[0])
	name := args[1]

	rec, err := telemetry.RecordInvocation("", etype, name)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error recording invocation: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Recorded call for [%s] %s -> Lifetime Calls: %d (Tier: %s)\n", etype, name, rec.CallCount, rec.Tier)
}

func runHook(args []string) {
	if len(args) == 2 && args[0] == "cli" {
		if _, err := telemetry.RecordObservedInvocation("", types.TypeCLI, args[1], "path-shim"); err != nil {
			fmt.Fprintln(os.Stderr, "ash cli hook:", err)
		}
		return
	}
	if len(args) != 1 || args[0] != "claude" {
		fmt.Fprintln(os.Stderr, "Usage: ash hook claude | ash hook cli <name>")
		os.Exit(1)
	}
	// Hooks must never block the user's agent. Fail open while logging errors.
	input, err := io.ReadAll(io.LimitReader(os.Stdin, 8<<20))
	if err != nil {
		fmt.Fprintln(os.Stderr, "ash hook read:", err)
		return
	}
	obs, valid, err := instrument.ParseClaudeHook(input)
	if err != nil {
		fmt.Fprintln(os.Stderr, "ash hook decode:", err)
		return
	}
	if !valid {
		return
	}
	if _, err = telemetry.RecordObservedInvocation("", obs.Type, obs.Name, obs.Source); err != nil {
		fmt.Fprintln(os.Stderr, "ash hook record:", err)
	}
}

func runInstrument(args []string) {
	if len(args) < 2 || (args[0] != "claude" && args[0] != "cli") || (args[1] != "install" && args[1] != "status") {
		fmt.Fprintln(os.Stderr, "Usage: ash instrument claude install|status OR ash instrument cli install|status [names...]")
		os.Exit(1)
	}
	binary, err := os.Executable()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	binary, err = filepath.Abs(binary)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if args[0] == "cli" {
		shims := instrument.CLIShimsDir()
		if args[1] == "status" {
			fmt.Println("CLI shims:", shims)
			fmt.Println("PATH starts with shims:", strings.HasPrefix(os.Getenv("PATH"), shims+string(os.PathListSeparator)))
			return
		}
		if len(args) < 3 {
			fmt.Fprintln(os.Stderr, "Specify CLI names, e.g. ash instrument cli install claude gh bun")
			os.Exit(1)
		}
		for _, name := range args[2:] {
			wrapper, err := instrument.InstallCLIShim(shims, binary, name)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Cannot instrument %s: %v\n", name, err)
				os.Exit(1)
			}
			fmt.Println("Installed CLI tracking shim:", wrapper)
		}
		changed, err := instrument.EnableZshShims("", shims)
		if err != nil {
			fmt.Fprintln(os.Stderr, "Shims installed, PATH setup failed:", err)
			os.Exit(1)
		}
		if changed {
			fmt.Println("Updated ~/.zshrc with backup ~/.zshrc.ash-backup")
		}
		fmt.Println("Open a new terminal; agents need to inherit the updated PATH to use CLI shims")
		return
	}
	if args[1] == "install" {
		backup, err := instrument.InstallClaudeHooks("", binary)
		if err != nil {
			fmt.Fprintln(os.Stderr, "Claude hook installation failed:", err)
			os.Exit(1)
		}
		if backup != "" {
			fmt.Println("Original settings backup:", backup)
		}
	}
	enabled, err := instrument.ClaudeHooksInstalled("", binary)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("Claude Code recording: %v\nHooks: PostToolUse + PostToolUseFailure (MCP/Skill), SubagentStart (agents)\n", enabled)
}

func runDedupe(args []string) {
	fs := flag.NewFlagSet("dedupe", flag.ExitOnError)
	dryRun := fs.Bool("dry-run", false, "Preview deduplication without mutating disk")
	apply := fs.Bool("apply", false, "Archive identical source folders and replace with canonical symlinks")
	_ = fs.Parse(args)
	preview := *dryRun || !*apply

	fmt.Printf("=== AGentic Skills Healer: Safe Duplicate Review ===\n")
	if preview {
		fmt.Println("[PREVIEW] No files will be modified. Use --apply to archive identical copies and link them")
	}

	res, err := healer.Dedupe(preview)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Deduplication error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Total Scanned in ~/.gemini/config/skills: %d\n", res.TotalScanned)
	fmt.Printf("Already Symlinks:                        %d\n", res.AlreadyLinks)
	fmt.Printf("Identical Copies %s: %d\n", map[bool]string{true: "Eligible", false: "Archived & Linked"}[preview], res.Converted)
	fmt.Printf("Different Content Kept: %d\n", res.SkippedDifferent)
	fmt.Printf("Unsafe or Backup Entries Skipped: %d\n", res.SkippedUnsafe)
	for _, backup := range res.Backups {
		fmt.Printf("Recovery copy retained: %s\n", backup)
	}
	if len(res.Errors) > 0 {
		fmt.Printf("Errors encountered (%d):\n", len(res.Errors))
		for _, e := range res.Errors {
			fmt.Printf("  ! %s\n", e)
		}
	} else if preview {
		fmt.Println("Preview complete. Matching names with different content were left untouched")
	} else {
		fmt.Println("Done. Original copies remain in the recovery vault")
	}
}

func runQuiet(args []string) {
	fs := flag.NewFlagSet("quiet", flag.ExitOnError)
	dryRun := fs.Bool("dry-run", false, "Preview quiet mode without moving files")
	apply := fs.Bool("apply", false, "Move eligible symbolic links to standby after review")
	minCalls := fs.Int("min-calls", 10, "Minimum calls for a skill to remain active (default: 10)")
	_ = fs.Parse(args)
	preview := *dryRun || !*apply

	db, _ := telemetry.Load("")
	fmt.Printf("=== AGentic Skills Healer: Standby Review ===\n")
	if preview {
		fmt.Printf("[PREVIEW] No files will be changed (min-calls: %d). Use --apply to move eligible aliases\n", *minCalls)
	}

	res, err := healer.Quiet(preview, *minCalls, db)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Quiet mode error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Total Skills Before:       %d\n", res.TotalBefore)
	fmt.Printf("Kept in Active Set (HOT):   %d\n", res.KeptActive)
	fmt.Printf("Eligible Standby Moves:    %d\n", res.MovedStandby)
	fmt.Printf("Skipped Without Changes:   %d\n", res.Skipped)
	for _, message := range res.Errors {
		fmt.Printf("  ! %s\n", message)
	}
}

func runLoad(args []string) {
	if len(args) < 1 {
		fmt.Println("Usage: ash load <skill-name>")
		os.Exit(1)
	}
	skillName := args[0]
	t0 := time.Now()
	err := healer.Load(skillName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error mounting skill: %v\n", err)
		os.Exit(1)
	}
	// Mounting a skill is a configuration change, not evidence of usage.
	fmt.Printf("Mounted skill '%s' into Active Set in %s (Ready for use by any agent)\n", skillName, time.Since(t0))
}

func runUnload(args []string) {
	if len(args) < 1 {
		fmt.Println("Usage: ash unload <skill-name>")
		os.Exit(1)
	}
	skillName := args[0]
	err := healer.Unload(skillName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error unmounting skill: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Unmounted skill '%s' back to Standby Vault.\n", skillName)
}

func runDoctor(args []string) {
	home, _ := os.UserHomeDir()
	geminiSkills := filepath.Join(home, ".gemini", "config", "skills")

	fmt.Printf("=== AGentic Skills Healer: System Doctor ===\n")
	entries, err := os.ReadDir(geminiSkills)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Cannot read %s: %v\n", geminiSkills, err)
		os.Exit(1)
	}

	brokenLinks := 0
	validLinks := 0
	realDirs := 0

	for _, e := range entries {
		p := filepath.Join(geminiSkills, e.Name())
		lInfo, lErr := os.Lstat(p)
		if lErr != nil {
			continue
		}
		if lInfo.Mode()&os.ModeSymlink != 0 {
			target, _ := os.Readlink(p)
			if _, statErr := os.Stat(p); statErr != nil {
				brokenLinks++
				fmt.Printf("  [BROKEN] %s -> %s\n", e.Name(), target)
			} else {
				validLinks++
			}
		} else {
			realDirs++
		}
	}

	fmt.Printf("Active Set Integrity:\n")
	fmt.Printf("  • Valid Symlinks:   %d\n", validLinks)
	fmt.Printf("  • Real Directories: %d\n", realDirs)
	fmt.Printf("  • Broken Symlinks:  %d\n", brokenLinks)
	if brokenLinks == 0 {
		fmt.Println("✓ All links and references are healthy and green!")
	}
}
