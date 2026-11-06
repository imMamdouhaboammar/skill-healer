package healer

import (
	"os"
	"path/filepath"
	"testing"
)

func skillFixture(t *testing.T, path, manifest string) {
	t.Helper()
	if err := os.MkdirAll(path, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "SKILL.md"), []byte(manifest), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestDedupeAtNeverReplacesDifferentSameNameSkills(t *testing.T) {
	root := t.TempDir()
	gemini, canonical, vault := filepath.Join(root, "gemini"), filepath.Join(root, "canonical"), filepath.Join(root, "vault")
	skillFixture(t, filepath.Join(gemini, "audit"), "Gemini-specific procedure")
	skillFixture(t, filepath.Join(canonical, "audit"), "Different canonical procedure")
	result, err := DedupeAt(gemini, canonical, vault, false)
	if err != nil {
		t.Fatal(err)
	}
	if result.Converted != 0 || result.SkippedDifferent != 1 {
		t.Fatalf("should skip distinct content, got %+v", result)
	}
	if info, err := os.Lstat(filepath.Join(gemini, "audit")); err != nil || !info.IsDir() {
		t.Fatalf("original skill was altered: %v %+v", err, info)
	}
	data, err := os.ReadFile(filepath.Join(gemini, "audit", "SKILL.md"))
	if err != nil || string(data) != "Gemini-specific procedure" {
		t.Fatalf("original content lost: %s %v", data, err)
	}
}

func TestDedupeAtArchivesIdenticalCopyBeforeSymlinking(t *testing.T) {
	root := t.TempDir()
	gemini, canonical, vault := filepath.Join(root, "gemini"), filepath.Join(root, "canonical"), filepath.Join(root, "vault")
	skillFixture(t, filepath.Join(gemini, "audit"), "Same procedure")
	skillFixture(t, filepath.Join(canonical, "audit"), "Same procedure")
	dry, err := DedupeAt(gemini, canonical, vault, true)
	if err != nil || dry.Converted != 1 {
		t.Fatalf("incorrect dry-run: %+v %v", dry, err)
	}
	if info, err := os.Lstat(filepath.Join(gemini, "audit")); err != nil || !info.IsDir() {
		t.Fatal("dry-run modified original")
	}
	result, err := DedupeAt(gemini, canonical, vault, false)
	if err != nil {
		t.Fatal(err)
	}
	if result.Converted != 1 || len(result.Backups) != 1 {
		t.Fatalf("copy must be retained for recovery: %+v", result)
	}
	target, err := os.Readlink(filepath.Join(gemini, "audit"))
	if err != nil || target != filepath.Join(canonical, "audit") {
		t.Fatalf("expected canonical symlink, got %s %v", target, err)
	}
	backup, err := os.ReadFile(filepath.Join(result.Backups[0], "SKILL.md"))
	if err != nil || string(backup) != "Same procedure" {
		t.Fatalf("backup missing: %s %v", backup, err)
	}
	second, err := DedupeAt(gemini, canonical, vault, false)
	if err != nil || second.AlreadyLinks != 1 || second.Converted != 0 {
		t.Fatalf("repeated dedupe should be idempotent: %+v %v", second, err)
	}
}

func TestDedupeAtPreservesStaleNamedFoldersAndRejectsCanonicalLinks(t *testing.T) {
	root := t.TempDir()
	gemini, canonical, vault := filepath.Join(root, "gemini"), filepath.Join(root, "canonical"), filepath.Join(root, "vault")
	skillFixture(t, filepath.Join(gemini, "audit.bak"), "Old backup should survive")
	skillFixture(t, filepath.Join(gemini, "linked"), "Canonical link is untrusted")
	source := filepath.Join(root, "real")
	skillFixture(t, source, "Canonical link is untrusted")
	if err := os.MkdirAll(canonical, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(source, filepath.Join(canonical, "linked")); err != nil {
		t.Fatal(err)
	}
	result, err := DedupeAt(gemini, canonical, vault, false)
	if err != nil {
		t.Fatal(err)
	}
	if result.Converted != 0 {
		t.Fatalf("unexpected deduplication: %+v", result)
	}
	for _, name := range []string{"audit.bak", "linked"} {
		if info, err := os.Lstat(filepath.Join(gemini, name)); err != nil || !info.IsDir() {
			t.Fatalf("unsafe replacement of %s: %v", name, err)
		}
	}
}

func TestQuietDoesNotOverwriteAnExistingStandbyCopy(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	active := filepath.Join(home, ".gemini", "config", "skills")
	standby := filepath.Join(home, ".ash", "vault", "standby")
	source := filepath.Join(home, ".agents", "skills", "review")
	skillFixture(t, source, "User review")
	if err := os.MkdirAll(active, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(standby, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(source, filepath.Join(active, "review")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(standby, "review"), []byte("previous recovery"), 0600); err != nil {
		t.Fatal(err)
	}
	result, err := Quiet(false, 10, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.MovedStandby != 0 {
		t.Fatalf("must not claim an unsafe move completed: %+v", result)
	}
	data, err := os.ReadFile(filepath.Join(standby, "review"))
	if err != nil || string(data) != "previous recovery" {
		t.Fatalf("previous standby lost: %s %v", data, err)
	}
	if _, err := os.Lstat(filepath.Join(active, "review")); err != nil {
		t.Fatal("active link must survive collision", err)
	}
}

func TestUnloadNeverOverwritesExistingStandby(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	active := filepath.Join(home, ".gemini", "config", "skills", "demo")
	standby := filepath.Join(home, ".ash", "vault", "standby", "demo")
	skillFixture(t, filepath.Join(home, ".agents", "skills", "demo"), "Demo")
	if err := os.MkdirAll(filepath.Dir(active), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(standby), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(home, ".agents", "skills", "demo"), active); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(standby, []byte("retain"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := Unload("demo"); err == nil {
		t.Fatal("collision must fail")
	}
	if data, err := os.ReadFile(standby); err != nil || string(data) != "retain" {
		t.Fatalf("standby changed: %s %v", data, err)
	}
	if _, err := os.Lstat(active); err != nil {
		t.Fatal("active copy missing", err)
	}
}

func TestLoadNeverOverwritesEvenAnEmptyActiveDirectory(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	skillFixture(t, filepath.Join(home, ".agents", "skills", "demo"), "Demo")
	active := filepath.Join(home, ".gemini", "config", "skills", "demo")
	if err := os.MkdirAll(active, 0700); err != nil {
		t.Fatal(err)
	}
	if err := Load("demo"); err == nil {
		t.Fatal("active directory must be kept")
	}
	if info, err := os.Lstat(active); err != nil || !info.IsDir() {
		t.Fatalf("active directory modified: %+v %v", info, err)
	}
}

func TestMountCommandsRejectTraversalNames(t *testing.T) {
	for _, name := range []string{"../other", "../../Documents", ".hidden", "unsafe/name", ""} {
		if err := Load(name); err == nil {
			t.Errorf("load accepted unsafe name %q", name)
		}
		if err := Unload(name); err == nil {
			t.Errorf("unload accepted unsafe name %q", name)
		}
	}
}
