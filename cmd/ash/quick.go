package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"text/tabwriter"

	"github.com/imMamdouhaboammar/skill-healer/pkg/dashboard"
)

func runQuick(args []string) {
	flags := flag.NewFlagSet("quick", flag.ExitOnError)
	asJSON := flags.Bool("json", false, "Output saved controls as machine-readable JSON")
	_ = flags.Parse(args)
	state, err := dashboard.ReadSavedState()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Could not read local saved controls: %v\n", err)
		os.Exit(1)
	}
	if err := writeQuick(os.Stdout, state, *asJSON); err != nil {
		fmt.Fprintf(os.Stderr, "Quick access output failed: %v\n", err)
		os.Exit(1)
	}
}

func writeQuick(output io.Writer, state dashboard.UserState, asJSON bool) error {
	if asJSON {
		encoder := json.NewEncoder(output)
		encoder.SetIndent("", "  ")
		return encoder.Encode(state)
	}
	w := tabwriter.NewWriter(output, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "AGENTIC SKILLS HEALER / QUICK ACCESS")
	fmt.Fprintln(w, "\nPINNED TOOLS")
	ids := make([]string, 0)
	for id, preference := range state.Tools {
		if preference.Favorite {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	for _, id := range ids {
		fmt.Fprintf(w, "  %s\t%s\n", id, state.Tools[id].Alias)
	}
	if len(ids) == 0 {
		fmt.Fprintln(w, "  None saved")
	}
	fmt.Fprintln(w, "\nSAVED COMMANDS (copy and review before executing)")
	ids = ids[:0]
	for id := range state.Shortcuts {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		shortcut := state.Shortcuts[id]
		fmt.Fprintf(w, "  %s\t%s\n", shortcut.Label, shortcut.Command)
	}
	if len(ids) == 0 {
		fmt.Fprintln(w, "  None saved")
	}
	fmt.Fprintln(w, "\nCOLLECTIONS")
	ids = ids[:0]
	for id := range state.Presets {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		preset := state.Presets[id]
		fmt.Fprintf(w, "  %s\t%d tool references\n", preset.Name, len(preset.ToolIDs))
	}
	if len(ids) == 0 {
		fmt.Fprintln(w, "  None saved")
	}
	fmt.Fprintf(w, "\nSTANDBY ALIASES\t%d\n", len(state.Standby))
	return w.Flush()
}
