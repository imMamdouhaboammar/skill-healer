package scanner

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/imMamdouhaboammar/skill-healer/pkg/types"
)

func TestAgentsAreDiscoveredAndObservedNotInvented(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PATH", filepath.Join(home, "empty"))
	agents := filepath.Join(home, ".claude", "agents")
	if err := os.MkdirAll(agents, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(agents, "reviewer.md"), []byte("---\nname: reviewer\n---"), 0600); err != nil {
		t.Fatal(err)
	}
	db := &types.TelemetryDB{Entities: map[string]types.TelemetryRecord{
		"agent:Explore": {Name: "agent:Explore", Type: "agent", CallCount: 12, VerifiedCalls: 3, Tier: "HOT"},
	}}
	pool, err := ScanPool(db)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := pool.Agents["self"]; ok {
		t.Fatal("invented a virtual agent")
	}
	if item, ok := pool.Agents["reviewer"]; !ok || item.Path == "" || item.Scope != "global" {
		t.Fatalf("real agent missing: %+v", item)
	}
	if item, ok := pool.Agents["Explore"]; !ok || item.VerifiedCalls != 3 || item.Scope != "observed_runtime" {
		t.Fatalf("observed runtime agent missing: %+v", item)
	}
	if len(pool.Agents) != 2 {
		t.Fatalf("unexpected agents: %+v", pool.Agents)
	}
}
