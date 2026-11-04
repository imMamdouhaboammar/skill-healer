package scanner

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/imMamdouhaboammar/skill-healer/pkg/classifier"
	"github.com/imMamdouhaboammar/skill-healer/pkg/telemetry"
	"github.com/imMamdouhaboammar/skill-healer/pkg/types"
)

// ScanPool performs a full system scan of all Skills, MCPs, CLIs, and Agents.
func ScanPool(db *types.TelemetryDB) (*types.Pool, error) {
	home, _ := os.UserHomeDir()

	pool := &types.Pool{
		Skills:    make(map[string]types.Entity),
		MCPs:      make(map[string]types.Entity),
		CLIs:      make(map[string]types.Entity),
		Agents:    make(map[string]types.Entity),
		ScannedAt: time.Now(),
	}

	// 1. Scan Skills
	skillDirs := []struct {
		Path  string
		Scope string
	}{
		{filepath.Join(home, ".gemini", "config", "skills"), "global"},
		{filepath.Join(home, ".agents", "skills"), "canonical"},
		{filepath.Join(home, ".codex", "skills"), "global"},
	}

	for _, sd := range skillDirs {
		entries, err := os.ReadDir(sd.Path)
		if err != nil {
			continue
		}
		for _, e := range entries {
			name := e.Name()
			if strings.HasPrefix(name, ".") {
				continue
			}
			fullPath := filepath.Join(sd.Path, name)
			isSymlink := false
			symlinkTarget := ""

			lInfo, lErr := os.Lstat(fullPath)
			if lErr == nil && (lInfo.Mode()&os.ModeSymlink != 0) {
				isSymlink = true
				if tgt, rErr := os.Readlink(fullPath); rErr == nil {
					symlinkTarget = tgt
				}
			}

			// Telemetry lookup
			var callCount int64 = 0
			tier := types.TierDormant
			var firstSeen, lastSeen *time.Time

			if db != nil {
				if rec, ok := telemetry.GetRecord(db, types.TypeSkill, name); ok {
					callCount = rec.CallCount
					tier = types.Tier(rec.Tier)
					if rec.FirstSeen > 0 {
						t1 := time.Unix(int64(rec.FirstSeen), 0)
						firstSeen = &t1
					}
					if rec.LastSeen > 0 {
						t2 := time.Unix(int64(rec.LastSeen), 0)
						lastSeen = &t2
					}
				}
			}

			ent := types.Entity{
				ID:            "skill:" + name,
				Name:          name,
				Type:          types.TypeSkill,
				Domain:        classifier.ClassifyName(name),
				Path:          fullPath,
				IsSymlink:     isSymlink,
				SymlinkTarget: symlinkTarget,
				Scope:         sd.Scope,
				CallCount:     callCount,
				FirstSeen:     firstSeen,
				LastSeen:      lastSeen,
				Tier:          tier,
				Status:        "active",
			}
			pool.Skills[name] = ent
		}
	}

	// 2. Scan MCPs
	mcpConfigs := []string{
		filepath.Join(home, ".gemini", "antigravity", "mcp_config.json"),
		filepath.Join(home, ".claude.json"),
		filepath.Join(home, ".claude", "settings.json"),
	}

	for _, mc := range mcpConfigs {
		data, err := os.ReadFile(mc)
		if err != nil {
			continue
		}
		var parsed struct {
			MCPServers map[string]interface{} `json:"mcpServers"`
		}
		if err := json.Unmarshal(data, &parsed); err == nil {
			for sname := range parsed.MCPServers {
				var callCount int64 = 0
				tier := types.TierDormant
				if db != nil {
					if rec, ok := telemetry.GetRecord(db, types.TypeMCP, sname); ok {
						callCount = rec.CallCount
						tier = types.Tier(rec.Tier)
					}
				}

				pool.MCPs[sname] = types.Entity{
					ID:        "mcp:" + sname,
					Name:      sname,
					Type:      types.TypeMCP,
					Domain:    classifier.ClassifyName(sname),
					Path:      mc,
					Scope:     "global",
					CallCount: callCount,
					Tier:      tier,
					Status:    "configured",
				}
			}
		}
	}

	// Check installed MCP tool bundles in ~/.gemini/antigravity/mcp/
	mcpStorage := filepath.Join(home, ".gemini", "antigravity", "mcp")
	if entries, err := os.ReadDir(mcpStorage); err == nil {
		for _, e := range entries {
			if e.IsDir() {
				sname := e.Name()
				if _, exists := pool.MCPs[sname]; !exists {
					var callCount int64 = 0
					tier := types.TierDormant
					if db != nil {
						if rec, ok := telemetry.GetRecord(db, types.TypeMCP, sname); ok {
							callCount = rec.CallCount
							tier = types.Tier(rec.Tier)
						}
					}
					pool.MCPs[sname] = types.Entity{
						ID:        "mcp:" + sname,
						Name:      sname,
						Type:      types.TypeMCP,
						Domain:    classifier.ClassifyName(sname),
						Path:      filepath.Join(mcpStorage, sname),
						Scope:     "installed_bundle",
						CallCount: callCount,
						Tier:      tier,
						Status:    "available",
					}
				}
			}
		}
	}

	// 3. Scan CLIs in PATH
	pathEnv := os.Getenv("PATH")
	paths := filepath.SplitList(pathEnv)
	cliFilter := map[string]bool{
		"claude": true, "codex": true, "get-fable": true, "fable-dsh": true,
		"agent-kernel": true, "no-mistakes": true, "gbrain": true, "xerj": true,
		"riqor": true, "fastmcp": true, "rea": true, "rea-agents": true, "agentapi": true,
		"agent-browser": true, "bun": true, "cargo": true, "rustc": true, "go": true,
		"python3": true, "docker": true, "kubectl": true, "gh": true, "git": true,
		"lazygit": true, "gitleaks": true, "playwright": true, "tiny-agents": true,
	}

	for _, p := range paths {
		entries, err := os.ReadDir(p)
		if err != nil {
			continue
		}
		for _, e := range entries {
			name := e.Name()
			if cliFilter[name] || strings.HasPrefix(name, "agent-kernel-") || strings.HasPrefix(name, "hostinger-") {
				if _, exists := pool.CLIs[name]; !exists {
					var callCount int64 = 0
					tier := types.TierDormant
					if db != nil {
						if rec, ok := telemetry.GetRecord(db, types.TypeCLI, name); ok {
							callCount = rec.CallCount
							tier = types.Tier(rec.Tier)
						}
					}

					pool.CLIs[name] = types.Entity{
						ID:        "cli:" + name,
						Name:      name,
						Type:      types.TypeCLI,
						Domain:    classifier.ClassifyName(name),
						Path:      filepath.Join(p, name),
						Scope:     "system",
						CallCount: callCount,
						Tier:      tier,
						Status:    "executable",
					}
				}
			}
		}
	}

	// 4. Scan Agents
	agentsList := []string{
		"self", "research", "image-generator", "firestore-rules-author",
		"fable-orchestrator", "agent-kernel-supervisor", "codex-subagent",
	}
	for _, aname := range agentsList {
		var callCount int64 = 0
		tier := types.TierDormant
		if db != nil {
			if rec, ok := telemetry.GetRecord(db, types.TypeAgent, aname); ok {
				callCount = rec.CallCount
				tier = types.Tier(rec.Tier)
			}
		}
		pool.Agents[aname] = types.Entity{
			ID:        "agent:" + aname,
			Name:      aname,
			Type:      types.TypeAgent,
			Domain:    types.DomainOrchestration,
			Path:      "system:subagent",
			Scope:     "runtime",
			CallCount: callCount,
			Tier:      tier,
			Status:    "ready",
		}
	}

	pool.Total = len(pool.Skills) + len(pool.MCPs) + len(pool.CLIs) + len(pool.Agents)
	return pool, nil
}
