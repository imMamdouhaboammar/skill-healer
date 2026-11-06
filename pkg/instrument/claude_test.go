package instrument

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestParseClaudeHook(t *testing.T) {
	cases := []struct {
		name, payload, kind, entity string
		valid                       bool
	}{
		{"mcp", `{"hook_event_name":"PostToolUse","tool_name":"mcp__github__search_repositories"}`, "mcp", "github", true},
		{"skill", `{"hook_event_name":"PostToolUse","tool_name":"Skill","tool_input":{"skill":"get-fable"}}`, "skill", "get-fable", true},
		{"agent", `{"hook_event_name":"SubagentStart","agent_type":"Explore"}`, "agent", "Explore", true},
		{"denied", `{"hook_event_name":"PreToolUse","tool_name":"mcp__github__search"}`, "", "", false},
		{"failed", `{"hook_event_name":"PostToolUseFailure","tool_name":"mcp__github__search"}`, "mcp", "github", true},
		{"bash", `{"hook_event_name":"PostToolUse","tool_name":"Bash"}`, "", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok, err := ParseClaudeHook([]byte(tc.payload))
			if err != nil || ok != tc.valid || (ok && (string(got.Type) != tc.kind || got.Name != tc.entity)) {
				t.Fatalf("got %+v, %v, %v", got, ok, err)
			}
		})
	}
}

func TestInstallClaudeHooksPreservesSettings(t *testing.T) {
	dir := t.TempDir()
	binary := filepath.Join(dir, "ash")
	if err := os.WriteFile(binary, []byte("fixture"), 0700); err != nil {
		t.Fatal(err)
	}
	settings := filepath.Join(dir, "claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(settings), 0700); err != nil {
		t.Fatal(err)
	}
	original := []byte(`{"enabledPlugins":{"example":true},"hooks":{"PostToolUse":[{"matcher":"Write","hooks":[{"type":"command","command":"printf existing"}]}]}}`)
	if err := os.WriteFile(settings, original, 0600); err != nil {
		t.Fatal(err)
	}
	backup, err := InstallClaudeHooks(settings, binary)
	if err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(backup)
	if err != nil || string(content) != string(original) {
		t.Fatalf("invalid backup: %v", err)
	}
	enabled, err := ClaudeHooksInstalled(settings, binary)
	if err != nil || !enabled {
		t.Fatalf("hook not installed: %v", err)
	}
	first, err := os.ReadFile(settings)
	if err != nil {
		t.Fatal(err)
	}
	_, err = InstallClaudeHooks(settings, binary)
	if err != nil {
		t.Fatal(err)
	}
	next, err := os.ReadFile(settings)
	if err != nil || string(first) != string(next) {
		t.Fatal("second installation was not idempotent")
	}
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(first, &doc); err != nil {
		t.Fatal(err)
	}
	if _, ok := doc["enabledPlugins"]; !ok {
		t.Fatal("lost prior setting")
	}
	if !containsHookFromRaw(doc, "PostToolUse", "printf existing") {
		t.Fatal("lost existing hook")
	}
	hooks, err := readHooks(doc)
	if err != nil {
		t.Fatal(err)
	}
	matched := false
	for _, group := range hooks["PostToolUse"] {
		if !containsHook([]map[string]json.RawMessage{group}, hookCommand(binary)) {
			continue
		}
		var expr string
		_ = json.Unmarshal(group["matcher"], &expr)
		matched = expr == "Skill|mcp__.*"
	}
	if !matched {
		t.Fatal("Claude post-tool hook is too broad")
	}
}

func containsHookFromRaw(doc map[string]json.RawMessage, event, command string) bool {
	hooks, err := readHooks(doc)
	return err == nil && containsHook(hooks[event], command)
}
