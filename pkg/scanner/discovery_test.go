package scanner

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/imMamdouhaboammar/skill-healer/pkg/types"
)

func fixtureFile(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
}

func fixtureHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PATH", filepath.Join(home, "no-binaries"))
	t.Setenv("ASH_PROJECT_ROOTS", "")
	return home
}

func TestGlobalSkillSourcesAndRealManifest(t *testing.T) {
	home := fixtureHome(t)
	fixtureFile(t, filepath.Join(home, ".claude", "skills", "reviewer", "SKILL.md"), "---\ndescription: Reviews security issues\n---\n", 0600)
	fixtureFile(t, filepath.Join(home, ".gemini", "skills", "writer", "SKILL.md"), "# Marketing copywriting", 0600)
	fixtureFile(t, filepath.Join(home, ".codex", "skills", "remote-skills", "nested", "SKILL.md"), "# Docker operations", 0600)
	fixtureFile(t, filepath.Join(home, ".agents", "skills", "not-a-skill", "notes.txt"), "no manifest", 0600)
	pool, err := ScanPool(nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"reviewer", "writer", "nested"} {
		if _, ok := pool.Skills[name]; !ok {
			t.Errorf("missed valid installed skill %q", name)
		}
	}
	if _, ok := pool.Skills["not-a-skill"]; ok {
		t.Error("accepted an entry without SKILL.md")
	}
}

func TestProjectDiscoveryAndMCPHostFormats(t *testing.T) {
	home := fixtureHome(t)
	project := filepath.Join(home, "Documents", "client", "product")
	t.Setenv("ASH_PROJECT_ROOTS", filepath.Join(home, "Documents"))
	fixtureFile(t, filepath.Join(project, ".claude", "skills", "local-skill", "SKILL.md"), "---\ndescription: Security inspection\n---\n", 0600)
	fixtureFile(t, filepath.Join(project, ".claude", "agents", "quality.md"), "---\nname: quality\ndescription: Review pull requests\n---", 0600)
	fixtureFile(t, filepath.Join(project, ".mcp.json"), `{"mcpServers":{"project-search":{"command":"search"}}}`, 0600)
	fixtureFile(t, filepath.Join(home, ".codex", "config.toml"), "[mcp_servers.global-search]\ncommand = \"global-search\"\n", 0600)
	fixtureFile(t, filepath.Join(home, ".gemini", "settings.json"), `{"mcpServers":{"gemini-native":{"command":"server"}}}`, 0600)
	fixtureFile(t, filepath.Join(home, ".claude.json"), `{"projects":{"/tmp/example":{"mcpServers":{"claude-project":{"command":"server"}}}}}`, 0600)
	pool, err := ScanPool(nil)
	if err != nil {
		t.Fatal(err)
	}
	skill, ok := pool.Skills["local-skill"]
	if !ok || skill.Scope != "project" {
		t.Fatalf("project skill not found: %+v", skill)
	}
	if _, ok := pool.Agents["quality"]; !ok {
		t.Error("project agent not found")
	}
	for _, name := range []string{"project-search", "global-search", "gemini-native", "claude-project"} {
		if _, ok := pool.MCPs[name]; !ok {
			t.Errorf("missed configured MCP server %q", name)
		}
	}
}

func TestDuplicateSkillNamesRetainEveryInstallation(t *testing.T) {
	home := fixtureHome(t)
	fixtureFile(t, filepath.Join(home, ".agents", "skills", "audit", "SKILL.md"), "# Security audit", 0600)
	fixtureFile(t, filepath.Join(home, ".claude", "skills", "audit", "SKILL.md"), "# Security audit", 0600)
	pool, err := ScanPool(&types.TelemetryDB{Entities: map[string]types.TelemetryRecord{
		"skill:audit": {Name: "skill:audit", Type: "skill", CallCount: 20, VerifiedCalls: 8, Tier: "HOT"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]bool{}
	paths := map[string]bool{}
	count := 0
	for _, item := range pool.Skills {
		if item.Name != "audit" {
			continue
		}
		count++
		if ids[item.ID] {
			t.Error("duplicate installation has duplicate ID")
		}
		ids[item.ID] = true
		paths[item.Path] = true
	}
	if count != 2 || len(paths) != 2 {
		t.Fatalf("lost duplicate installation: count=%d paths=%v", count, paths)
	}
}

func TestSameNameWithoutTelemetryStillHasAmbiguousAttribution(t *testing.T) {
	home := fixtureHome(t)
	fixtureFile(t, filepath.Join(home, ".agents", "skills", "review", "SKILL.md"), "# Security review", 0600)
	fixtureFile(t, filepath.Join(home, ".claude", "skills", "review", "SKILL.md"), "# Different review procedure", 0600)
	pool, err := ScanPool(nil)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, item := range pool.Skills {
		if item.Name != "review" {
			continue
		}
		count++
		if item.Attribution != "ambiguous_name" {
			t.Errorf("unrecorded shared name incorrectly looks attributable: %+v", item)
		}
	}
	if count != 2 {
		t.Fatalf("expected two same-name entries, got %d", count)
	}
}

func TestUserCLIsAndExecutableValidation(t *testing.T) {
	home := fixtureHome(t)
	bin := filepath.Join(home, ".local", "bin")
	fixtureFile(t, filepath.Join(bin, "custom-agent-command"), "#!/bin/sh\nexit 0\n", 0700)
	fixtureFile(t, filepath.Join(bin, "looks-executable"), "data", 0600)
	t.Setenv("PATH", bin)
	pool, err := ScanPool(nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := pool.CLIs["custom-agent-command"]; !ok {
		t.Fatal("user installed CLI omitted")
	}
	if _, ok := pool.CLIs["looks-executable"]; ok {
		t.Fatal("non-executable file accepted as CLI")
	}
}

func TestSameMCPNameInTwoClaudeProjectsRemainsSeparate(t *testing.T) {
	home := fixtureHome(t)
	fixtureFile(t, filepath.Join(home, ".claude.json"), `{"projects":{"/work/alpha":{"mcpServers":{"memory":{"command":"one"}}},"/work/beta":{"mcpServers":{"memory":{"command":"two"}}}}}`, 0600)
	pool, err := ScanPool(nil)
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]bool{}
	count := 0
	for _, item := range pool.MCPs {
		if item.Name != "memory" {
			continue
		}
		count++
		if ids[item.ID] {
			t.Fatal("reused MCP registration identity")
		}
		ids[item.ID] = true
	}
	if count != 2 {
		t.Fatalf("collapsed two separate project registrations: %d", count)
	}
}

func TestDuplicateIdentityStableAcrossRepeatedScans(t *testing.T) {
	home := fixtureHome(t)
	fixtureFile(t, filepath.Join(home, ".claude.json"), `{"projects":{"/work/zeta":{"mcpServers":{"shared":{"command":"one"}}},"/work/alpha":{"mcpServers":{"shared":{"command":"two"}}},"/work/beta":{"mcpServers":{"shared":{"command":"three"}}}}}`, 0600)
	baseline := map[string]string{}
	for i := 0; i < 30; i++ {
		pool, err := ScanPool(nil)
		if err != nil {
			t.Fatal(err)
		}
		for _, item := range pool.MCPs {
			if item.Name != "shared" {
				continue
			}
			if old, ok := baseline[item.Source]; ok && old != item.ID {
				t.Fatalf("source %s changed identity from %s to %s", item.Source, old, item.ID)
			}
			baseline[item.Source] = item.ID
		}
	}
	if len(baseline) != 3 {
		t.Fatalf("expected three distinct project registrations: %+v", baseline)
	}
}
