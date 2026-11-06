package instrument

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/imMamdouhaboammar/skill-healer/pkg/types"
)

type ObservedEvent struct {
	Type   types.EntityType
	Name   string
	Source string
}

// ParseClaudeHook counts completed and failed tool executions, plus initiated agents.
func ParseClaudeHook(payload []byte) (ObservedEvent, bool, error) {
	var event map[string]json.RawMessage
	if err := json.Unmarshal(payload, &event); err != nil {
		return ObservedEvent{}, false, fmt.Errorf("parsing Claude hook: %w", err)
	}
	field := func(key string) string { var s string; _ = json.Unmarshal(event[key], &s); return strings.TrimSpace(s) }
	switch field("hook_event_name") {
	case "PostToolUse", "PostToolUseFailure":
		tool := field("tool_name")
		source := "claude-code"
		if field("hook_event_name") == "PostToolUseFailure" {
			source = "claude-code/failed"
		}
		if strings.HasPrefix(tool, "mcp__") {
			parts := strings.SplitN(strings.TrimPrefix(tool, "mcp__"), "__", 2)
			if len(parts) == 2 && parts[0] != "" && parts[1] != "" {
				return ObservedEvent{Type: types.TypeMCP, Name: parts[0], Source: source}, true, nil
			}
		}
		if tool == "Skill" {
			var input map[string]json.RawMessage
			_ = json.Unmarshal(event["tool_input"], &input)
			var skill string
			_ = json.Unmarshal(input["skill"], &skill)
			if strings.TrimSpace(skill) != "" {
				return ObservedEvent{Type: types.TypeSkill, Name: strings.TrimSpace(skill), Source: source}, true, nil
			}
		}
	case "SubagentStart":
		if agent := field("agent_type"); agent != "" {
			return ObservedEvent{Type: types.TypeAgent, Name: agent, Source: "claude-code"}, true, nil
		}
	}
	return ObservedEvent{}, false, nil
}

func ClaudeSettingsPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".claude", "settings.json")
}

func shellQuote(path string) string    { return "'" + strings.ReplaceAll(path, "'", "'\\''") + "'" }
func hookCommand(binary string) string { return shellQuote(binary) + " hook claude" }
func hookEvents() []string             { return []string{"PostToolUse", "PostToolUseFailure", "SubagentStart"} }

type hookDef struct {
	Matcher string
	Hooks   []hookHandler
}
type hookHandler struct {
	Type    string
	Command string
}

func containsHook(groups []map[string]json.RawMessage, command string) bool {
	for _, group := range groups {
		var handlers []hookHandler
		_ = json.Unmarshal(group["hooks"], &handlers)
		for _, h := range handlers {
			if h.Type == "command" && h.Command == command {
				return true
			}
		}
	}
	return false
}

func readHooks(settings map[string]json.RawMessage) (map[string][]map[string]json.RawMessage, error) {
	hooks := map[string][]map[string]json.RawMessage{}
	if raw, ok := settings["hooks"]; ok {
		if err := json.Unmarshal(raw, &hooks); err != nil {
			return nil, fmt.Errorf("unsupported Claude hooks format: %w", err)
		}
	}
	if hooks == nil {
		hooks = map[string][]map[string]json.RawMessage{}
	}
	return hooks, nil
}

// InstallClaudeHooks merges hooks without deleting pre-existing settings or matchers.
// A fixed backup preserves the original settings for recovery.
func InstallClaudeHooks(settingsPath, ashBinary string) (string, error) {
	if !filepath.IsAbs(ashBinary) {
		return "", errors.New("ash binary must be an absolute path")
	}
	fi, err := os.Stat(ashBinary)
	if err != nil || !fi.Mode().IsRegular() || fi.Mode().Perm()&0111 == 0 {
		return "", fmt.Errorf("ash binary must be executable: %s", ashBinary)
	}
	if settingsPath == "" {
		settingsPath = ClaudeSettingsPath()
	}
	if settingsPath == "" {
		return "", errors.New("home directory unavailable")
	}
	original, err := os.ReadFile(settingsPath)
	exists := err == nil
	if err != nil && !os.IsNotExist(err) {
		return "", err
	}
	settings := map[string]json.RawMessage{}
	if exists {
		if err = json.Unmarshal(original, &settings); err != nil {
			return "", fmt.Errorf("invalid Claude settings, unchanged: %w", err)
		}
	}
	hooks, err := readHooks(settings)
	if err != nil {
		return "", err
	}
	command := hookCommand(ashBinary)
	changed := false
	for _, event := range hookEvents() {
		if containsHook(hooks[event], command) {
			if event == "PostToolUse" || event == "PostToolUseFailure" {
				for _, group := range hooks[event] {
					if !containsHook([]map[string]json.RawMessage{group}, command) {
						continue
					}
					var matcherValue string
					_ = json.Unmarshal(group["matcher"], &matcherValue)
					var handlers []json.RawMessage
					_ = json.Unmarshal(group["hooks"], &handlers)
					if matcherValue == "" && len(handlers) == 1 {
						group["matcher"] = json.RawMessage(`"Skill|mcp__.*"`)
						changed = true
					}
				}
			}
			continue
		}
		matcher := map[string]json.RawMessage{}
		matcher["hooks"] = json.RawMessage(fmt.Sprintf("[{\"type\":\"command\",\"command\":%q,\"timeout\":15}]", command))
		if event == "PostToolUse" || event == "PostToolUseFailure" {
			matcher["matcher"] = json.RawMessage(`"Skill|mcp__.*"`)
		}
		hooks[event] = append(hooks[event], matcher)
		changed = true
	}
	if !changed {
		return "", nil
	}
	raw, err := json.Marshal(hooks)
	if err != nil {
		return "", err
	}
	settings["hooks"] = raw
	updated, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return "", err
	}
	if err = os.MkdirAll(filepath.Dir(settingsPath), 0700); err != nil {
		return "", err
	}
	backup := ""
	if exists {
		backup = settingsPath + ".ash-backup"
		if _, err = os.Stat(backup); os.IsNotExist(err) {
			if err = os.WriteFile(backup, original, 0600); err != nil {
				return "", fmt.Errorf("backing up Claude settings: %w", err)
			}
		} else if err != nil {
			return "", fmt.Errorf("checking Claude backup: %w", err)
		}
	}
	tmp, err := os.CreateTemp(filepath.Dir(settingsPath), ".ash-claude-*.json")
	if err != nil {
		return backup, err
	}
	defer os.Remove(tmp.Name())
	if _, err = tmp.Write(updated); err != nil {
		_ = tmp.Close()
		return backup, err
	}
	if err = tmp.Chmod(0600); err != nil {
		_ = tmp.Close()
		return backup, err
	}
	if err = tmp.Sync(); err != nil {
		_ = tmp.Close()
		return backup, err
	}
	if err = tmp.Close(); err != nil {
		return backup, err
	}
	if err = os.Rename(tmp.Name(), settingsPath); err != nil {
		return backup, err
	}
	return backup, nil
}

func ClaudeHooksInstalled(settingsPath, ashBinary string) (bool, error) {
	if settingsPath == "" {
		settingsPath = ClaudeSettingsPath()
	}
	original, err := os.ReadFile(settingsPath)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var settings map[string]json.RawMessage
	if err = json.Unmarshal(original, &settings); err != nil {
		return false, err
	}
	hooks, err := readHooks(settings)
	if err != nil {
		return false, err
	}
	for _, event := range hookEvents() {
		if !containsHook(hooks[event], hookCommand(ashBinary)) {
			return false, nil
		}
	}
	return true, nil
}
