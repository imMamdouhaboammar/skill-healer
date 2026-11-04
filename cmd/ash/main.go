package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/imMamdouhaboammar/skill-healer/pkg/healer"
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
  pool            Display full inventory of CLIs, MCPs, Skills, and Agents
  stats           Display Lifetime Lite Counter statistics and telemetry
  top             Display top most-invoked entities across all categories
  dormant         Display all dormant (unused) skills and MCPs
  doctor          Run health check on symlinks, paths, and configurations

Healing & Noise Control:
  dedupe          Deduplicate redundant physical directories via canonical symlinks (--dry-run supported)
  quiet           Activate lean Active Set; move dormant skills to On-Demand Standby (--dry-run supported)
  load <skill>    Dynamically mount any skill into Active Set in 1ms
  unload <skill>  Unmount a skill from Active Set back to Standby

Telemetry:
  track <t> <n>   Record an invocation (t = cli|mcp|skill|agent, n = name)
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
	var totalCalls int64 = 0

	for _, rec := range db.Entities {
		typeCounts[rec.Type]++
		tierCounts[rec.Tier]++
		totalCalls += rec.CallCount
	}

	fmt.Printf("Lifetime Invocations: %d calls across all tools\n\n", totalCalls)
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

func runDedupe(args []string) {
	fs := flag.NewFlagSet("dedupe", flag.ExitOnError)
	dryRun := fs.Bool("dry-run", false, "Preview deduplication without mutating disk")
	_ = fs.Parse(args)

	fmt.Printf("=== AGentic Skills Healer: Deduplication Engine ===\n")
	if *dryRun {
		fmt.Println("[DRY-RUN MODE] No files will be modified.")
	}

	res, err := healer.Dedupe(*dryRun)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Deduplication error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Total Scanned in ~/.gemini/config/skills: %d\n", res.TotalScanned)
	fmt.Printf("Already Symlinks:                        %d\n", res.AlreadyLinks)
	fmt.Printf("Converted Real Directories to Symlinks:  %d\n", res.Converted)
	fmt.Printf("Stale Backup Artifacts Cleaned:          %d\n", res.StaleRemoved)
	if len(res.Errors) > 0 {
		fmt.Printf("Errors encountered (%d):\n", len(res.Errors))
		for _, e := range res.Errors {
			fmt.Printf("  ! %s\n", e)
		}
	} else {
		fmt.Println("All duplicate directories successfully unified with canonical store!")
	}
}

func runQuiet(args []string) {
	fs := flag.NewFlagSet("quiet", flag.ExitOnError)
	dryRun := fs.Bool("dry-run", false, "Preview quiet mode without moving files")
	minCalls := fs.Int("min-calls", 10, "Minimum calls for a skill to remain active (default: 10)")
	_ = fs.Parse(args)

	db, _ := telemetry.Load("")
	fmt.Printf("=== AGentic Skills Healer: Quiet Mode Optimization ===\n")
	if *dryRun {
		fmt.Printf("[DRY-RUN MODE] Previewing Active Set vs Standby (min-calls: %d)...\n", *minCalls)
	}

	res, err := healer.Quiet(*dryRun, *minCalls, db)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Quiet mode error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Total Skills Before:       %d\n", res.TotalBefore)
	fmt.Printf("Kept in Active Set (HOT):   %d\n", res.KeptActive)
	fmt.Printf("Moved to Standby Vault:    %d\n", res.MovedStandby)
	noiseCut := float64(res.MovedStandby) / float64(res.TotalBefore) * 100
	fmt.Printf("Context Prompt Noise Cut:  %.1f%%\n", noiseCut)
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
	// Record telemetry
	_, _ = telemetry.RecordInvocation("", types.TypeSkill, skillName)
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
