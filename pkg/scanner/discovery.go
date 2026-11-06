package scanner

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/imMamdouhaboammar/skill-healer/pkg/classifier"
	"github.com/imMamdouhaboammar/skill-healer/pkg/telemetry"
	"github.com/imMamdouhaboammar/skill-healer/pkg/types"
)

type collector struct {
	pool   *types.Pool
	db     *types.TelemetryDB
	seen   map[string]bool
	groups map[string][]string
}

func newCollector(db *types.TelemetryDB) *collector {
	return &collector{
		pool: &types.Pool{
			Skills: map[string]types.Entity{}, MCPs: map[string]types.Entity{},
			CLIs: map[string]types.Entity{}, Agents: map[string]types.Entity{},
			ScannedAt: time.Now(),
		},
		db: db, seen: map[string]bool{}, groups: map[string][]string{},
	}
}

func fingerprintID(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:8])
}

func (c *collector) table(kind types.EntityType) map[string]types.Entity {
	switch kind {
	case types.TypeSkill:
		return c.pool.Skills
	case types.TypeMCP:
		return c.pool.MCPs
	case types.TypeCLI:
		return c.pool.CLIs
	default:
		return c.pool.Agents
	}
}

func (c *collector) add(kind types.EntityType, name, path, scope, source, description, status string) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 240 {
		return
	}
	path = filepath.Clean(path)
	key := string(kind) + "\x00" + path + "\x00" + scope + "\x00" + source + "\x00" + name
	if c.seen[key] {
		return
	}
	c.seen[key] = true
	table := c.table(kind)
	group := string(kind) + "\x00" + name
	id := string(kind) + ":" + name
	mapKey := name
	if len(c.groups[group]) > 0 {
		suffix := fingerprintID(key)
		id += "@" + suffix
		mapKey += "@" + suffix
	}
	result := classifier.ClassifyDetailed(name, description)
	item := types.Entity{
		ID: id, Name: name, Type: kind, Domain: result.Domain,
		Description: description, Confidence: result.Confidence, Basis: result.Basis,
		Path: path, Scope: scope, Source: source, Status: status, Tier: types.TierDormant,
	}
	if info, err := os.Lstat(path); err == nil && info.Mode()&os.ModeSymlink != 0 {
		item.IsSymlink = true
		item.SymlinkTarget, _ = os.Readlink(path)
	}
	if len(c.groups[group]) > 0 {
		first := table[c.groups[group][0]]
		item.DuplicateOf = first.ID
	}
	table[mapKey] = item
	c.groups[group] = append(c.groups[group], mapKey)
}

func (c *collector) applyTelemetry() {
	for group, keys := range c.groups {
		parts := strings.SplitN(group, "\x00", 2)
		if len(parts) != 2 {
			continue
		}
		kind, name := types.EntityType(parts[0]), parts[1]
		table := c.table(kind)
		// Even with no telemetry, an invocation by name cannot be assigned to one
		// of several installations. Keep all copies out of automatic cleanup hints.
		if len(keys) > 1 {
			for _, key := range keys {
				item := table[key]
				item.Attribution = "ambiguous_name"
				table[key] = item
			}
		}
		if c.db == nil {
			continue
		}
		record, found := telemetry.GetRecord(c.db, kind, name)
		if !found {
			continue
		}
		// Host hooks currently report a name, not a physical installation path.
		// Preserve aggregate counts once, and explicitly flag ambiguous attribution.
		for index, key := range keys {
			item := table[key]
			if len(keys) == 1 {
				item.Attribution = "name_match"
			}
			if index == 0 {
				item.CallCount, item.VerifiedCalls = record.CallCount, record.VerifiedCalls
				if record.Tier != "" {
					item.Tier = types.Tier(record.Tier)
				}
				if record.FirstSeen > 0 {
					v := time.Unix(int64(record.FirstSeen), 0)
					item.FirstSeen = &v
				}
				if record.LastSeen > 0 {
					v := time.Unix(int64(record.LastSeen), 0)
					item.LastSeen = &v
				}
			}
			table[key] = item
		}
	}
}

func (c *collector) discoveredRuntimeAgents() {
	if c.db == nil {
		return
	}
	for key, record := range c.db.Entities {
		if record.Type != string(types.TypeAgent) || record.VerifiedCalls == 0 {
			continue
		}
		name := strings.TrimPrefix(key, "agent:")
		group := string(types.TypeAgent) + "\x00" + name
		if len(c.groups[group]) > 0 {
			continue
		}
		c.add(types.TypeAgent, name, "runtime:"+name, "observed_runtime", "telemetry", "", "observed")
	}
}

func (c *collector) recordRoot(path string) {
	if path == "" {
		return
	}
	if len(c.pool.Coverage.Scanned) < 256 {
		c.pool.Coverage.Scanned = append(c.pool.Coverage.Scanned, path)
	}
}

func (c *collector) warn(format string, args ...any) {
	if len(c.pool.Coverage.Warnings) < 80 {
		c.pool.Coverage.Warnings = append(c.pool.Coverage.Warnings, fmt.Sprintf(format, args...))
	}
}

func ScanPool(db *types.TelemetryDB) (*types.Pool, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	c := newCollector(db)
	c.scanGlobal(home)
	c.scanCLIs(home)
	c.scanProjects(home)
	c.discoveredRuntimeAgents()
	c.applyTelemetry()
	c.pool.Total = len(c.pool.Skills) + len(c.pool.MCPs) + len(c.pool.CLIs) + len(c.pool.Agents)
	return c.pool, nil
}

func (c *collector) scanGlobal(home string) {
	for _, spec := range []struct{ root, source string }{
		{filepath.Join(home, ".agents", "skills"), "agents"},
		{filepath.Join(home, ".codex", "skills"), "codex"},
		{filepath.Join(home, ".gemini", "config", "skills"), "gemini-legacy"},
		{filepath.Join(home, ".claude", "skills"), "claude"},
		{filepath.Join(home, ".gemini", "skills"), "gemini"},
		{filepath.Join(home, ".cursor", "skills"), "cursor"},
	} {
		c.scanSkillRoot(spec.root, "global", spec.source)
	}
	for _, spec := range []struct{ root, source string }{
		{filepath.Join(home, ".claude", "agents"), "claude"},
		{filepath.Join(home, ".agents", "agents"), "agents"},
		{filepath.Join(home, ".codex", "agents"), "codex"},
		{filepath.Join(home, ".gemini", "agents"), "gemini"},
		{filepath.Join(home, ".cursor", "agents"), "cursor"},
	} {
		c.scanAgentRoot(spec.root, "global", spec.source)
	}
	for _, spec := range []struct{ path, source string }{
		{filepath.Join(home, ".gemini", "antigravity", "mcp_config.json"), "gemini-antigravity"},
		{filepath.Join(home, ".gemini", "settings.json"), "gemini"},
		{filepath.Join(home, ".claude.json"), "claude"},
		{filepath.Join(home, ".claude", "settings.json"), "claude"},
		{filepath.Join(home, ".cursor", "mcp.json"), "cursor"},
		{filepath.Join(home, ".copilot", "mcp-config.json"), "copilot"},
		{filepath.Join(home, ".codex", "config.toml"), "codex"},
	} {
		c.scanMCPConfig(spec.path, "global", spec.source)
	}
	store := filepath.Join(home, ".gemini", "antigravity", "mcp")
	if entries, err := os.ReadDir(store); err == nil {
		for _, entry := range entries {
			if entry.IsDir() {
				c.add(types.TypeMCP, entry.Name(), filepath.Join(store, entry.Name()), "global", "gemini-bundle", "", "available")
			}
		}
	}
}
