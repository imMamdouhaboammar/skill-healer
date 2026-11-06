package instrument

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLIShimPreservesInvocation(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "bin")
	shims := filepath.Join(dir, "shims")
	if err := os.MkdirAll(bin, 0700); err != nil {
		t.Fatal(err)
	}
	original := filepath.Join(bin, "fixture")
	if err := os.WriteFile(original, []byte("#!/bin/sh\nprintf 'original:%s|%s\\n' \"$1\" \"$2\"\nexit 19\n"), 0700); err != nil {
		t.Fatal(err)
	}
	ash := filepath.Join(bin, "ash")
	if err := os.WriteFile(ash, []byte("#!/bin/sh\nprintf 'hook:%s|%s\\n' \"$1\" \"$2\" > \"$ASH_TEST_LOG\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	log := filepath.Join(dir, "calls.log")
	t.Setenv("PATH", bin)
	t.Setenv("ASH_TEST_LOG", log)
	shim, err := InstallCLIShim(shims, ash, "fixture")
	if err != nil {
		t.Fatal(err)
	}
	output, err := exec.Command(shim, "two words", "second").CombinedOutput()
	if string(output) != "original:two words|second\n" {
		t.Fatalf("arguments corrupted: %q", output)
	}
	if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != 19 {
		t.Fatalf("exit status was not preserved: %v", err)
	}
	traced, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	if string(traced) != "hook:hook|cli\n" {
		t.Fatalf("hook missing: %q", traced)
	}
}

func TestZshPATHSetupIsReversibleAndIdempotent(t *testing.T) {
	dir := t.TempDir()
	rc := filepath.Join(dir, ".zshrc")
	initial := []byte("export HISTSIZE=5000\n")
	if err := os.WriteFile(rc, initial, 0600); err != nil {
		t.Fatal(err)
	}
	added, err := EnableZshShims(rc, filepath.Join(dir, "shims"))
	if err != nil || !added {
		t.Fatalf("enable failed: %v", err)
	}
	backup, err := os.ReadFile(rc + ".ash-backup")
	if err != nil || string(backup) != string(initial) {
		t.Fatal("original shell config was not backed up")
	}
	added, err = EnableZshShims(rc, filepath.Join(dir, "shims"))
	if err != nil || added {
		t.Fatalf("duplicate shell entry: %v", err)
	}
	content, err := os.ReadFile(rc)
	if err != nil || strings.Count(string(content), zshMarker) != 1 {
		t.Fatal("unexpected PATH content")
	}
}
