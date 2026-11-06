package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imMamdouhaboammar/skill-healer/pkg/dashboard"
)

func TestCleanupCLIPlansRemovesAndRestoresRealFixture(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	source := filepath.Join(home, ".agents", "skills", "test-ash-cleanup")
	if err := os.MkdirAll(source, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "SKILL.md"), []byte("keep this"), 0600); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := cleanupCommand([]string{"plan", "--id", "skill:test-ash-cleanup"}, &output); err != nil {
		t.Fatal(err)
	}
	var plan dashboard.CleanupPlan
	if err := json.Unmarshal(output.Bytes(), &plan); err != nil {
		t.Fatal(err)
	}
	if !plan.Allowed || plan.Fingerprint == "" {
		t.Fatalf("unexpected plan: %+v", plan)
	}
	output.Reset()
	if err := cleanupCommand([]string{"remove", "--id", plan.ID, "--fingerprint", plan.Fingerprint, "--confirm", "wrong"}, &output); err == nil {
		t.Fatal("unsafe confirmation accepted")
	}
	if _, err := os.Stat(source); err != nil {
		t.Fatalf("invalid confirmation moved source: %v", err)
	}
	output.Reset()
	if err := cleanupCommand([]string{"remove", "--id", plan.ID, "--fingerprint", plan.Fingerprint, "--confirm", plan.Name}, &output); err != nil {
		t.Fatal(err)
	}
	var record dashboard.TrashRecord
	if err := json.Unmarshal(output.Bytes(), &record); err != nil {
		t.Fatal(err)
	}
	if record.ID == "" {
		t.Fatal("missing trash id")
	}
	if _, err := os.Lstat(source); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("source still installed: %v", err)
	}
	output.Reset()
	if err := cleanupCommand([]string{"trash"}, &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), record.ID) {
		t.Fatal("missing trash record")
	}
	output.Reset()
	if err := cleanupCommand([]string{"restore", "--id", record.ID}, &output); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(filepath.Join(source, "SKILL.md"))
	if err != nil || string(content) != "keep this" {
		t.Fatalf("restore failed: %v, %q", err, content)
	}
}
