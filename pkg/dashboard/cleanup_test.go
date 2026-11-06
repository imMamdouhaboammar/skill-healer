package dashboard

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imMamdouhaboammar/skill-healer/pkg/types"
)

// A fake "cleanup" implementation would leave the installed skill in place.
func TestCleanupQuarantinesRealSkillAndRestoresItsContents(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, ".agents", "skills", "sample-skill")
	if err := os.MkdirAll(path, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "SKILL.md"), []byte("# Keep this exact content"), 0600); err != nil {
		t.Fatal(err)
	}
	store := newStore(filepath.Join(home, ".ash", "db", "dashboard.json"), home)
	cleaner := newCleanupManager(store)
	item := types.Entity{ID: "skill:sample-skill", Name: "sample-skill", Type: types.TypeSkill, Path: path, Scope: "canonical"}
	plan, err := cleaner.Plan(item)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Allowed || plan.Operation != "quarantine-path" || plan.Fingerprint == "" {
		t.Fatalf("invalid plan: %+v", plan)
	}
	if _, err := cleaner.Remove(item, plan.Fingerprint, "other-name"); !errors.Is(err, ErrUnsafe) {
		t.Fatalf("wrong confirmation accepted: %v", err)
	}
	record, err := cleaner.Remove(item, plan.Fingerprint, item.Name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("skill remains installed: %v", err)
	}
	content, err := os.ReadFile(filepath.Join(record.VaultPath, "payload", "SKILL.md"))
	if err != nil || string(content) != "# Keep this exact content" {
		t.Fatalf("quarantine contents wrong: %v", err)
	}
	if err := cleaner.Restore(record.ID); err != nil {
		t.Fatal(err)
	}
	content, err = os.ReadFile(filepath.Join(path, "SKILL.md"))
	if err != nil || string(content) != "# Keep this exact content" {
		t.Fatalf("restoration wrong: %v", err)
	}
	view, _ := store.View()
	if len(view.Trash) != 0 {
		t.Fatal("restored record remains in trash")
	}
}

func TestCleanupRemovesExactMCPConfigEntryAndRestoresWithoutOtherChanges(t *testing.T) {
	home := t.TempDir()
	configPath := filepath.Join(home, ".claude.json")
	original := `{"mcpServers":{"alpha":{"command":"alpha"},"beta":{"command":"beta"}},"preferences":{"theme":"light"}}`
	if err := os.WriteFile(configPath, []byte(original), 0600); err != nil {
		t.Fatal(err)
	}
	store := newStore(filepath.Join(home, ".ash", "db", "dashboard.json"), home)
	cleaner := newCleanupManager(store)
	item := types.Entity{ID: "mcp:alpha", Name: "alpha", Type: types.TypeMCP, Path: configPath, Scope: "global"}
	plan, err := cleaner.Plan(item)
	if err != nil || !plan.Allowed || plan.Operation != "remove-mcp-entry" {
		t.Fatalf("invalid MCP plan: %+v err=%v", plan, err)
	}
	record, err := cleaner.Remove(item, plan.Fingerprint, item.Name)
	if err != nil {
		t.Fatal(err)
	}
	result, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	var parsed map[string]json.RawMessage
	if err := json.Unmarshal(result, &parsed); err != nil {
		t.Fatal(err)
	}
	var servers map[string]json.RawMessage
	if err := json.Unmarshal(parsed["mcpServers"], &servers); err != nil {
		t.Fatal(err)
	}
	if _, exists := servers["alpha"]; exists {
		t.Fatal("deleted MCP registration is still configured")
	}
	if _, exists := servers["beta"]; !exists {
		t.Fatal("unrelated MCP deleted")
	}
	if !strings.Contains(string(result), `"theme": "light"`) {
		t.Fatal("unrelated config key changed")
	}
	view, _ := store.View()
	encoded, _ := json.Marshal(view)
	if strings.Contains(string(encoded), `"command":"alpha"`) {
		t.Fatal("MCP config body leaked into API-facing metadata")
	}
	if err := cleaner.Restore(record.ID); err != nil {
		t.Fatal(err)
	}
	result, _ = os.ReadFile(configPath)
	if !strings.Contains(string(result), `"alpha"`) || !strings.Contains(string(result), `"beta"`) {
		t.Fatalf("MCP restore incomplete: %s", result)
	}
}

func TestCleanupCLIAndAgentProvenanceRestrictions(t *testing.T) {
	home := t.TempDir()
	store := newStore(filepath.Join(home, ".ash", "db", "dashboard.json"), home)
	cleaner := newCleanupManager(store)
	for _, item := range []types.Entity{
		{ID: "cli:git", Name: "git", Type: types.TypeCLI, Path: "/usr/bin/git", Scope: "system"},
		{ID: "agent:virtual", Name: "virtual", Type: types.TypeAgent, Path: "system:subagent", Scope: "runtime"},
		{ID: "skill:outside", Name: "outside", Type: types.TypeSkill, Path: filepath.Join(home, "Documents", "outside"), Scope: "global"},
	} {
		plan, err := cleaner.Plan(item)
		if err != nil {
			t.Fatal(err)
		}
		if plan.Allowed || plan.Reason == "" {
			t.Fatalf("unsafe removal advertised as available: %+v", plan)
		}
	}
	path := filepath.Join(home, ".local", "bin", "my-local-cli")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0700); err != nil {
		t.Fatal(err)
	}
	item := types.Entity{ID: "cli:my-local-cli", Name: "my-local-cli", Type: types.TypeCLI, Path: path, Scope: "system"}
	plan, err := cleaner.Plan(item)
	if err != nil || !plan.Allowed {
		t.Fatalf("user-owned CLI cannot be removed: %+v %v", plan, err)
	}
	if _, err := cleaner.Remove(item, plan.Fingerprint, item.Name); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("CLI still active after cleanup")
	}
}

func TestCleanupNeverConfusesProjectMCPWithGlobalRegistration(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, ".claude.json")
	doc := `{"mcpServers":{"shared":{"command":"global"}},"projects":{"/work/client":{"mcpServers":{"shared":{"command":"project"}}}}}`
	if err := os.WriteFile(path, []byte(doc), 0600); err != nil {
		t.Fatal(err)
	}
	cleaner := newCleanupManager(newStore(filepath.Join(home, ".ash", "db", "dashboard.json"), home))
	item := types.Entity{ID: "mcp:shared@project", Name: "shared", Type: types.TypeMCP, Path: path, Scope: "project", Source: "claude:/work/client"}
	plan, err := cleaner.Plan(item)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Allowed {
		t.Fatal("project registration could delete global registration with the same name")
	}
}

func TestCleanupRecognizesGlobalClaudeSkill(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, ".claude", "skills", "security-review")
	if err := os.MkdirAll(path, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "SKILL.md"), []byte("# Security review"), 0600); err != nil {
		t.Fatal(err)
	}
	cleaner := newCleanupManager(newStore(filepath.Join(home, ".ash", "db", "dashboard.json"), home))
	item := types.Entity{ID: "skill:security-review", Name: "security-review", Type: types.TypeSkill, Path: path, Scope: "global", Source: "claude"}
	plan, err := cleaner.Plan(item)
	if err != nil || !plan.Allowed || plan.Operation != "quarantine-path" {
		t.Fatalf("eligible user-owned skill was blocked: %+v %v", plan, err)
	}
}

func TestCleanupAllowsExactProjectSkillAndMCPRegistration(t *testing.T) {
	home := t.TempDir()
	store := newStore(filepath.Join(home, ".ash", "db", "dashboard.json"), home)
	cleaner := newCleanupManager(store)
	skill := filepath.Join(home, "Documents", "product", ".claude", "skills", "reviewer")
	if err := os.MkdirAll(skill, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skill, "SKILL.md"), []byte("# Review"), 0600); err != nil {
		t.Fatal(err)
	}
	item := types.Entity{ID: "skill:reviewer@project", Name: "reviewer", Type: types.TypeSkill, Path: skill, Scope: "project", Source: "claude"}
	plan, err := cleaner.Plan(item)
	if err != nil || !plan.Allowed {
		t.Fatalf("project skill cannot be managed: %+v %v", plan, err)
	}
	config := filepath.Join(home, "Documents", "product", ".mcp.json")
	if err := os.WriteFile(config, []byte(`{"mcpServers":{"search":{"command":"server"}}}`), 0600); err != nil {
		t.Fatal(err)
	}
	mcp := types.Entity{ID: "mcp:search@project", Name: "search", Type: types.TypeMCP, Path: config, Scope: "project", Source: "claude"}
	plan, err = cleaner.Plan(mcp)
	if err != nil || !plan.Allowed || plan.Operation != "remove-mcp-entry" {
		t.Fatalf("project MCP not manageable: %+v %v", plan, err)
	}
}

func TestCleanupPurgeRequiresExplicitConfirmation(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, ".codex", "skills", "throwaway")
	if err := os.MkdirAll(path, 0700); err != nil {
		t.Fatal(err)
	}
	store := newStore(filepath.Join(home, ".ash", "db", "dashboard.json"), home)
	cleaner := newCleanupManager(store)
	item := types.Entity{ID: "skill:throwaway", Name: "throwaway", Type: types.TypeSkill, Path: path}
	plan, err := cleaner.Plan(item)
	if err != nil {
		t.Fatal(err)
	}
	record, err := cleaner.Remove(item, plan.Fingerprint, item.Name)
	if err != nil {
		t.Fatal(err)
	}
	if err := cleaner.Purge(record.ID, "throwaway"); !errors.Is(err, ErrUnsafe) {
		t.Fatalf("weak purge confirmation accepted: %v", err)
	}
	if _, err := os.Stat(record.VaultPath); err != nil {
		t.Fatal("invalid purge changed data")
	}
	if err := cleaner.Purge(record.ID, "DELETE throwaway"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(record.VaultPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("quarantine not permanently deleted: %v", err)
	}
	view, _ := store.View()
	if len(view.Trash) != 0 {
		t.Fatal("purged record still recorded")
	}
}

func TestCleanupRejectsStalePathPlanWithoutMovingCurrentFile(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, ".agents", "skills", "mutable")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("before"), 0600); err != nil {
		t.Fatal(err)
	}
	cleaner := newCleanupManager(newStore(filepath.Join(home, ".ash", "db", "dashboard.json"), home))
	item := types.Entity{ID: "skill:mutable", Name: "mutable", Path: path, Type: types.TypeSkill}
	plan, err := cleaner.Plan(item)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("changed, larger content"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := cleaner.Remove(item, plan.Fingerprint, item.Name); !errors.Is(err, ErrConflict) {
		t.Fatalf("expected stale plan rejection, got %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "changed, larger content" {
		t.Fatalf("new version must remain installed, got %q %v", data, err)
	}
}

func TestCleanupRejectsStaleMCPPlanWithoutRewritingConfiguration(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, ".claude.json")
	if err := os.WriteFile(path, []byte(`{"mcpServers":{"target":{"command":"old"}}}`), 0600); err != nil {
		t.Fatal(err)
	}
	cleaner := newCleanupManager(newStore(filepath.Join(home, ".ash", "db", "dashboard.json"), home))
	item := types.Entity{ID: "mcp:target", Name: "target", Path: path, Type: types.TypeMCP}
	plan, err := cleaner.Plan(item)
	if err != nil {
		t.Fatal(err)
	}
	updated := `{"mcpServers":{"target":{"command":"updated"}}}`
	if err := os.WriteFile(path, []byte(updated), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := cleaner.Remove(item, plan.Fingerprint, item.Name); !errors.Is(err, ErrConflict) {
		t.Fatalf("expected stale config rejection, got %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != updated {
		t.Fatalf("MCP config must stay intact, got %s %v", data, err)
	}
}

func TestCleanupRestoreRejectsExistingSourceWithoutOverwriting(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, ".codex", "skills", "restore-conflict")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("archived"), 0600); err != nil {
		t.Fatal(err)
	}
	store := newStore(filepath.Join(home, ".ash", "db", "dashboard.json"), home)
	cleaner := newCleanupManager(store)
	item := types.Entity{ID: "skill:restore-conflict", Name: "restore-conflict", Path: path, Type: types.TypeSkill}
	plan, err := cleaner.Plan(item)
	if err != nil {
		t.Fatal(err)
	}
	record, err := cleaner.Remove(item, plan.Fingerprint, item.Name)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("newer"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := cleaner.Restore(record.ID); !errors.Is(err, ErrConflict) {
		t.Fatalf("expected restore conflict, got %v", err)
	}
	current, err := os.ReadFile(path)
	if err != nil || string(current) != "newer" {
		t.Fatalf("restore overwrote existing source: %q %v", current, err)
	}
	archived, err := os.ReadFile(filepath.Join(record.VaultPath, "payload"))
	if err != nil || string(archived) != "archived" {
		t.Fatalf("failed restore lost archived data: %q %v", archived, err)
	}
}
