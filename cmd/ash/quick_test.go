package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/imMamdouhaboammar/skill-healer/pkg/dashboard"
)

func TestQuickJSONExposesSavedActionsForAgents(t *testing.T) {
	state := dashboard.UserState{
		Version:   1,
		Tools:     map[string]dashboard.ToolPreference{"skill:writer": {Alias: "Writer", Favorite: true}},
		Shortcuts: map[string]dashboard.Shortcut{"cmd-one": {ID: "cmd-one", Label: "Overview", Command: "ash stats"}},
		Presets:   map[string]dashboard.Preset{"preset-one": {ID: "preset-one", Name: "Daily", ToolIDs: []string{"skill:writer"}}},
		Standby:   map[string]dashboard.StandbyRecord{},
	}
	var output bytes.Buffer
	if err := writeQuick(&output, state, true); err != nil {
		t.Fatal(err)
	}
	var decoded dashboard.UserState
	if err := json.Unmarshal(output.Bytes(), &decoded); err != nil {
		t.Fatalf("agent-readable JSON invalid: %v", err)
	}
	if decoded.Shortcuts["cmd-one"].Command != "ash stats" || !decoded.Tools["skill:writer"].Favorite || decoded.Presets["preset-one"].Name != "Daily" {
		t.Fatalf("quick state missing saved controls: %+v", decoded)
	}
}

func TestQuickHumanOutputNamesSavedActionsWithoutExecutingThem(t *testing.T) {
	state := dashboard.UserState{Tools: map[string]dashboard.ToolPreference{}, Shortcuts: map[string]dashboard.Shortcut{"1": {Label: "List tools", Command: "ash pool --json"}}, Presets: map[string]dashboard.Preset{}, Standby: map[string]dashboard.StandbyRecord{}}
	var output bytes.Buffer
	if err := writeQuick(&output, state, false); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "ash pool --json") || !strings.Contains(output.String(), "List tools") {
		t.Fatalf("saved command missing: %s", output.String())
	}
}
