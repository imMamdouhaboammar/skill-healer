package scanner

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/imMamdouhaboammar/skill-healer/pkg/types"
)

var mcpTOMLSection = regexp.MustCompile(`^\s*\[mcp_servers\.(.+)\]\s*(?:#.*)?$`)

func jsonObject(raw json.RawMessage) map[string]json.RawMessage {
	values := map[string]json.RawMessage{}
	_ = json.Unmarshal(raw, &values)
	return values
}

func sortedKeys(values map[string]json.RawMessage) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func (c *collector) scanMCPConfig(path, scope, source string) {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return
	}
	if err != nil || !info.Mode().IsRegular() {
		c.warn("MCP config inaccessible: %s", path)
		return
	}
	if info.Size() > 4<<20 {
		c.warn("MCP config exceeds 4 MB parsing limit: %s", path)
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		c.warn("MCP config read failed: %s", path)
		return
	}
	c.recordRoot(path)
	if strings.EqualFold(filepath.Ext(path), ".toml") {
		scanner := bufio.NewScanner(strings.NewReader(string(data)))
		seen := map[string]bool{}
		for scanner.Scan() {
			match := mcpTOMLSection.FindStringSubmatch(scanner.Text())
			if len(match) != 2 {
				continue
			}
			name := strings.TrimSpace(match[1])
			if strings.HasPrefix(name, "\"") {
				name = strings.Trim(name, "\"")
			} else {
				name = strings.SplitN(name, ".", 2)[0]
			}
			if name == "" || seen[name] {
				continue
			}
			seen[name] = true
			c.add(types.TypeMCP, name, path, scope, source, "", "configured")
		}
		if err := scanner.Err(); err != nil {
			c.warn("TOML parsing interrupted: %s", path)
		}
		return
	}
	doc := map[string]json.RawMessage{}
	if err := json.Unmarshal(data, &doc); err != nil {
		c.warn("Malformed MCP config: %s", path)
		return
	}
	for _, field := range []string{"mcpServers", "servers"} {
		entries := jsonObject(doc[field])
		for _, name := range sortedKeys(entries) {
			raw := entries[name]
			description := ""
			fields := jsonObject(raw)
			_ = json.Unmarshal(fields["description"], &description)
			if len(description) > 240 {
				description = description[:240]
			}
			c.add(types.TypeMCP, name, path, scope, source, description, "configured")
		}
	}
	// Claude's machine-wide file also stores project-scoped MCP registrations
	// under projects[absoluteProjectPath].mcpServers
	projects := jsonObject(doc["projects"])
	for _, projectPath := range sortedKeys(projects) {
		rawProject := projects[projectPath]
		project := jsonObject(rawProject)
		for _, name := range sortedKeys(jsonObject(project["mcpServers"])) {
			c.add(types.TypeMCP, name, path, "project", source+":"+filepath.Clean(projectPath), "", "configured")
		}
	}
}
