package dashboard

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/imMamdouhaboammar/skill-healer/pkg/types"
)

func TestPreferencesAndShortcutsPersist(t *testing.T) {
	home := t.TempDir()
	store := newStore(filepath.Join(home, ".ash", "db", "dashboard.json"), home)
	if err := store.SetTool("skill:test", ToolPreference{Alias: "My editor", Favorite: true, Notes: "Review monthly", Tags: []string{"writing", "review"}}); err != nil {
		t.Fatal(err)
	}
	created, err := store.SaveShortcut(Shortcut{Label: "List agent tools", Command: "ash pool --json"})
	if err != nil {
		t.Fatal(err)
	}
	reopened := newStore(store.path, home)
	got, err := reopened.View()
	if err != nil {
		t.Fatal(err)
	}
	if !got.Tools["skill:test"].Favorite || got.Tools["skill:test"].Alias != "My editor" {
		t.Fatalf("metadata not persisted: %+v", got.Tools)
	}
	if len(got.Tools["skill:test"].Tags) != 2 {
		t.Fatalf("tags not persisted: %+v", got.Tools["skill:test"])
	}
	if got.Shortcuts[created.ID].Command != "ash pool --json" {
		t.Fatalf("shortcut not persisted: %+v", got.Shortcuts)
	}
	if err := reopened.DeleteShortcut(created.ID); err != nil {
		t.Fatal(err)
	}
	if err := reopened.DeleteTool("skill:test"); err != nil {
		t.Fatal(err)
	}
	after, _ := reopened.View()
	if len(after.Tools) != 0 || len(after.Shortcuts) != 0 {
		t.Fatalf("CRUD delete failed: %+v", after)
	}
	info, err := os.Stat(store.path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("state permissions = %v, want 0600", info.Mode())
	}
}

func TestPresetsCrudAndTagValidation(t *testing.T) {
	home := t.TempDir()
	store := newStore(filepath.Join(home, "state.json"), home)
	if err := store.SetTool("skill:test", ToolPreference{Tags: []string{"../../bad"}}); err == nil {
		t.Fatal("unsafe tag accepted")
	}
	preset, err := store.SavePreset(Preset{Name: "Daily writing", ToolIDs: []string{"skill:test", "cli:git"}})
	if err != nil {
		t.Fatal(err)
	}
	view, _ := store.View()
	if len(view.Presets) != 1 || len(view.Presets[preset.ID].ToolIDs) != 2 {
		t.Fatalf("preset not stored: %+v", view.Presets)
	}
	preset.Name = "Reviewed writing"
	preset.ToolIDs = []string{"skill:test"}
	if _, err := store.SavePreset(preset); err != nil {
		t.Fatal(err)
	}
	view, _ = store.View()
	if view.Presets[preset.ID].Name != "Reviewed writing" {
		t.Fatal("preset edit did not persist")
	}
	if err := store.DeletePreset(preset.ID); err != nil {
		t.Fatal(err)
	}
	view, _ = store.View()
	if len(view.Presets) != 0 {
		t.Fatal("preset delete did not persist")
	}
}

func TestStandbyMovesOnlyEligibleSymlinksAndRestores(t *testing.T) {
	home := t.TempDir()
	active := filepath.Join(home, ".gemini", "config", "skills")
	if err := os.MkdirAll(active, 0700); err != nil {
		t.Fatal(err)
	}
	canonical := filepath.Join(home, "canonical-skill")
	if err := os.Mkdir(canonical, 0700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(active, "editor-skill")
	if err := os.Symlink(canonical, link); err != nil {
		t.Fatal(err)
	}
	store := newStore(filepath.Join(home, ".ash", "db", "dashboard.json"), home)
	item := types.Entity{ID: "skill:editor", Name: "editor-skill", Type: types.TypeSkill, Path: link, IsSymlink: true}
	if !store.CanStandby(item) {
		t.Fatal("eligible symlink should be movable")
	}
	if err := store.MoveToStandby(item); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(link); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("active alias still present: %v", err)
	}
	if _, err := os.Stat(canonical); err != nil {
		t.Fatalf("canonical target changed: %v", err)
	}
	view, _ := store.View()
	if len(view.Standby) != 1 {
		t.Fatalf("expected standby record: %+v", view.Standby)
	}
	if err := store.RestoreFromStandby(item.ID); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Lstat(link); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("original symlink not restored: %v", err)
	}
	view, _ = store.View()
	if len(view.Standby) != 0 {
		t.Fatal("standby record not cleared")
	}
}

func TestStandbyRejectsCanonicalDirectoriesAndPathSpoofing(t *testing.T) {
	home := t.TempDir()
	store := newStore(filepath.Join(home, ".ash", "db", "dashboard.json"), home)
	path := filepath.Join(home, ".gemini", "config", "skills", "writer")
	if err := os.MkdirAll(path, 0700); err != nil {
		t.Fatal(err)
	}
	for _, item := range []types.Entity{
		{ID: "a", Name: "writer", Type: types.TypeSkill, Path: path, IsSymlink: false},
		{ID: "b", Name: "writer", Type: types.TypeSkill, Path: filepath.Join(home, "other", "writer"), IsSymlink: true},
		{ID: "c", Name: "wrong", Type: types.TypeSkill, Path: path, IsSymlink: true},
	} {
		if err := store.MoveToStandby(item); !errors.Is(err, ErrUnsafe) {
			t.Fatalf("unsafe item accepted: %+v, err=%v", item, err)
		}
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("directory was changed: %v", err)
	}
}
