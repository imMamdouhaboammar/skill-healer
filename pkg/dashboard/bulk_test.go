package dashboard

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imMamdouhaboammar/skill-healer/pkg/types"
)

func TestBulkCleanupRequiresPreviewAndTypedCountThenRestores(t *testing.T) {
	home := t.TempDir()
	pool := &types.Pool{Skills: make(map[string]types.Entity)}
	for _, name := range []string{"one", "two"} {
		path := filepath.Join(home, ".agents", "skills", name)
		if err := os.MkdirAll(path, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(path, "SKILL.md"), []byte("contents:"+name), 0600); err != nil {
			t.Fatal(err)
		}
		pool.Skills[name] = types.Entity{ID: "skill:" + name, Name: name, Type: types.TypeSkill, Path: path}
	}
	store := newStore(filepath.Join(home, ".ash", "db", "dashboard.json"), home)
	handler := handlerWithStore(func() (*types.Pool, error) { return pool, nil }, store, func(string) error { return nil })
	request := func(method, path, body, token string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "http://127.0.0.1:4317"+path, strings.NewReader(body))
		if token != "" {
			r.Header.Set("X-ASH-Session", token)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	token := request("GET", "/api/pool", "", "").Header().Get("X-ASH-Session")
	preview := request("POST", "/api/cleanup/bulk-plan", `{"ids":["skill:one","skill:two"]}`, token)
	if preview.Code != 200 {
		t.Fatalf("bulk preview failed: %d %s", preview.Code, preview.Body.String())
	}
	var plans []CleanupPlan
	if err := json.Unmarshal(preview.Body.Bytes(), &plans); err != nil {
		t.Fatal(err)
	}
	if len(plans) != 2 || !plans[0].Allowed || !plans[1].Allowed {
		t.Fatalf("bad preview: %+v", plans)
	}
	items, _ := json.Marshal([]map[string]string{{"id": plans[0].ID, "fingerprint": plans[0].Fingerprint}, {"id": plans[1].ID, "fingerprint": plans[1].Fingerprint}})
	bad := fmt.Sprintf(`{"items":%s,"confirmation":"REMOVE 1 TOOLS"}`, items)
	if resp := request("POST", "/api/cleanup/bulk-remove", bad, token); resp.Code == 200 {
		t.Fatal("incorrect count confirmed bulk deletion")
	}
	for _, name := range []string{"one", "two"} {
		if _, err := os.Stat(pool.Skills[name].Path); err != nil {
			t.Fatalf("source moved on bad confirmation: %v", err)
		}
	}
	good := fmt.Sprintf(`{"items":%s,"confirmation":"REMOVE 2 TOOLS"}`, items)
	if resp := request("POST", "/api/cleanup/bulk-remove", good, ""); resp.Code != http.StatusForbidden {
		t.Fatal("unguarded bulk removal accepted")
	}
	resp := request("POST", "/api/cleanup/bulk-remove", good, token)
	if resp.Code != 200 {
		t.Fatalf("bulk removal failed: %d %s", resp.Code, resp.Body.String())
	}
	var result BulkResult
	if err := json.Unmarshal(resp.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Completed != 2 || result.Failed != 0 {
		t.Fatalf("wrong result: %+v", result)
	}
	for _, name := range []string{"one", "two"} {
		if _, err := os.Lstat(pool.Skills[name].Path); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("source remains after batch")
		}
	}
	view, _ := store.View()
	if len(view.Trash) != 2 {
		t.Fatal("trash records not saved")
	}
	ids := make([]string, 0, 2)
	for id := range view.Trash {
		ids = append(ids, id)
	}
	body, _ := json.Marshal(map[string]any{"ids": ids})
	restore := request("POST", "/api/cleanup/bulk-restore", string(body), token)
	if restore.Code != 200 {
		t.Fatalf("bulk restore failed: %d %s", restore.Code, restore.Body.String())
	}
	for _, name := range []string{"one", "two"} {
		if _, err := os.Stat(filepath.Join(pool.Skills[name].Path, "SKILL.md")); err != nil {
			t.Fatal("bulk restore missing file")
		}
	}
}

func TestBulkCleanupRejectsUnsupportedItemBeforeAnyRemoval(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, ".agents", "skills", "safe")
	if err := os.MkdirAll(path, 0700); err != nil {
		t.Fatal(err)
	}
	pool := &types.Pool{Skills: map[string]types.Entity{"safe": {ID: "skill:safe", Name: "safe", Type: types.TypeSkill, Path: path}}, CLIs: map[string]types.Entity{"git": {ID: "cli:git", Name: "git", Type: types.TypeCLI, Path: "/usr/bin/git"}}}
	store := newStore(filepath.Join(home, ".ash", "db", "dashboard.json"), home)
	manager := newCleanupManager(store)
	plan, err := manager.Plan(pool.Skills["safe"])
	if err != nil {
		t.Fatal(err)
	}
	_, err = bulkRemove(pool, manager, BulkRemoveRequest{Items: []BulkItem{{ID: "skill:safe", Fingerprint: plan.Fingerprint}, {ID: "cli:git", Fingerprint: "unknown"}}, Confirmation: "REMOVE 2 TOOLS"})
	if !errors.Is(err, ErrUnsafe) {
		t.Fatalf("unsupported entry should block entire batch: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal("valid tool moved despite blocked batch")
	}
}

func TestBulkCleanupRemovesTwoMCPsFromOneConfig(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, ".claude.json")
	if err := os.WriteFile(path, []byte(`{"mcpServers":{"alpha":{"command":"a"},"beta":{"command":"b"},"keep":{"command":"k"}}}`), 0600); err != nil {
		t.Fatal(err)
	}
	pool := &types.Pool{MCPs: map[string]types.Entity{}}
	manager := newCleanupManager(newStore(filepath.Join(home, ".ash", "db", "dashboard.json"), home))
	items := make([]BulkItem, 0)
	for _, name := range []string{"alpha", "beta"} {
		ent := types.Entity{ID: "mcp:" + name, Name: name, Type: types.TypeMCP, Path: path}
		pool.MCPs[name] = ent
		plan, err := manager.Plan(ent)
		if err != nil {
			t.Fatal(err)
		}
		items = append(items, BulkItem{ID: ent.ID, Fingerprint: plan.Fingerprint})
	}
	result, err := bulkRemove(pool, manager, BulkRemoveRequest{Items: items, Confirmation: "REMOVE 2 TOOLS"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Completed != 2 || result.Failed != 0 {
		t.Fatalf("shared config bulk removal was partial: %+v", result)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), `"alpha"`) || strings.Contains(string(data), `"beta"`) || !strings.Contains(string(data), `"keep"`) {
		t.Fatalf("wrong config after bulk removal: %s", data)
	}
}

func TestBulkMetadataUpdatesPreserveExistingPreferences(t *testing.T) {
	home := t.TempDir()
	store := newStore(filepath.Join(home, ".ash", "db", "dashboard.json"), home)
	if err := store.SetTool("skill:one", ToolPreference{Alias: "My alias", Notes: "Preserve this", Tags: []string{"research"}}); err != nil {
		t.Fatal(err)
	}
	pool := &types.Pool{Skills: map[string]types.Entity{
		"one": {ID: "skill:one", Name: "one", Type: types.TypeSkill},
		"two": {ID: "skill:two", Name: "two", Type: types.TypeSkill},
	}}
	handler := handlerWithStore(func() (*types.Pool, error) { return pool, nil }, store, func(string) error { return nil })
	request := func(body, token string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "http://127.0.0.1:4317/api/tools/bulk", strings.NewReader(body))
		if token != "" {
			r.Header.Set("X-ASH-Session", token)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	initial := httptest.NewRecorder()
	handler.ServeHTTP(initial, httptest.NewRequest("GET", "http://127.0.0.1:4317/api/pool", nil))
	token := initial.Header().Get("X-ASH-Session")
	payload := `{"ids":["skill:one","skill:two"],"action":"hide"}`
	if response := request(payload, ""); response.Code != http.StatusForbidden {
		t.Fatal("unguarded metadata batch accepted")
	}
	if response := request(payload, token); response.Code != 200 {
		t.Fatalf("metadata bulk failed: %d %s", response.Code, response.Body.String())
	}
	view, err := store.View()
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"skill:one", "skill:two"} {
		if !view.Tools[id].Hidden {
			t.Fatal("item not hidden")
		}
	}
	if view.Tools["skill:one"].Alias != "My alias" || view.Tools["skill:one"].Notes != "Preserve this" || len(view.Tools["skill:one"].Tags) != 1 {
		t.Fatal("bulk operation erased other metadata")
	}
	if response := request(`{"ids":["skill:one","skill:two"],"action":"unhide"}`, token); response.Code != 200 {
		t.Fatal("bulk unhide failed")
	}
	view, _ = store.View()
	if view.Tools["skill:one"].Hidden || view.Tools["skill:two"].Hidden {
		t.Fatal("bulk unhide did not restore visibility")
	}
}

func TestBulkPurgeRequiresExactCountAndCleansRealVaults(t *testing.T) {
	home := t.TempDir()
	store := newStore(filepath.Join(home, ".ash", "db", "dashboard.json"), home)
	cleaner := newCleanupManager(store)
	ids := make([]string, 0, 2)
	for _, name := range []string{"first", "second"} {
		path := filepath.Join(home, ".agents", "skills", name)
		if err := os.MkdirAll(path, 0700); err != nil {
			t.Fatal(err)
		}
		item := types.Entity{ID: "skill:" + name, Name: name, Type: types.TypeSkill, Path: path}
		plan, err := cleaner.Plan(item)
		if err != nil {
			t.Fatal(err)
		}
		record, err := cleaner.Remove(item, plan.Fingerprint, name)
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, record.ID)
	}
	if _, err := bulkTrash(cleaner, ids, "purge", "DELETE 1 RECOVERY COPY"); !errors.Is(err, ErrUnsafe) {
		t.Fatalf("wrong confirmation accepted: %v", err)
	}
	view, _ := store.View()
	if len(view.Trash) != 2 {
		t.Fatal("invalid purge changed Trash")
	}
	result, err := bulkTrash(cleaner, ids, "purge", "DELETE 2 RECOVERY COPIES")
	if err != nil || result.Completed != 2 || result.Failed != 0 {
		t.Fatalf("bulk purge failed: %+v %v", result, err)
	}
	view, _ = store.View()
	if len(view.Trash) != 0 {
		t.Fatal("purged records still present")
	}
	for _, id := range ids {
		if _, err := os.Stat(filepath.Join(home, ".ash", "vault", "trash", id)); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("recovery copy remains after purge")
		}
	}
}

func TestBulkRejectsDuplicatesButAcceptsLargeSelections(t *testing.T) {
	if err := validateBulkIDs([]string{"one", "one"}); !errors.Is(err, ErrUnsafe) {
		t.Fatal("duplicate IDs accepted")
	}
	ids := make([]string, 1000)
	for i := range ids {
		ids[i] = fmt.Sprintf("skill:%d", i)
	}
	if err := validateBulkIDs(ids); err != nil {
		t.Fatalf("large selection rejected: %v", err)
	}
}

func TestBulkRemovalOfThirtyActualSkillDirectories(t *testing.T) {
	home := t.TempDir()
	pool := &types.Pool{Skills: map[string]types.Entity{}}
	manager := newCleanupManager(newStore(filepath.Join(home, ".ash", "db", "dashboard.json"), home))
	items := make([]BulkItem, 0, 30)
	for i := 0; i < 30; i++ {
		name := fmt.Sprintf("fixture-%02d", i)
		path := filepath.Join(home, ".agents", "skills", name)
		if err := os.MkdirAll(path, 0700); err != nil {
			t.Fatal(err)
		}
		item := types.Entity{ID: "skill:" + name, Name: name, Type: types.TypeSkill, Path: path}
		pool.Skills[name] = item
		plan, err := manager.Plan(item)
		if err != nil {
			t.Fatal(err)
		}
		items = append(items, BulkItem{ID: item.ID, Fingerprint: plan.Fingerprint})
	}
	result, err := bulkRemove(pool, manager, BulkRemoveRequest{Items: items, Confirmation: "REMOVE 30 TOOLS"})
	if err != nil || result.Completed != 30 || result.Failed != 0 {
		t.Fatalf("large real cleanup failed: %+v %v", result, err)
	}
	for _, item := range pool.Skills {
		if _, err := os.Lstat(item.Path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("source remains: %s", item.Path)
		}
	}
}

func TestBulkPlanAPIHandlesMoreThanEightKilobytes(t *testing.T) {
	home := t.TempDir()
	pool := &types.Pool{Skills: map[string]types.Entity{}}
	ids := make([]string, 0, 220)
	for i := 0; i < 220; i++ {
		name := fmt.Sprintf("sample-with-a-long-identifier-%04d-for-bulk-selection", i)
		id := "skill:" + name
		ids = append(ids, id)
		pool.Skills[name] = types.Entity{ID: id, Name: name, Type: types.TypeSkill, Path: filepath.Join(home, ".agents", "skills", name)}
	}
	payload, err := json.Marshal(BulkIDsRequest{IDs: ids})
	if err != nil {
		t.Fatal(err)
	}
	if len(payload) <= 8192 {
		t.Fatal("fixture must exceed the former small JSON body limit")
	}
	store := newStore(filepath.Join(home, ".ash", "db", "dashboard.json"), home)
	handler := handlerWithStore(func() (*types.Pool, error) { return pool, nil }, store, func(string) error { return nil })
	initial := httptest.NewRecorder()
	handler.ServeHTTP(initial, httptest.NewRequest("GET", "http://127.0.0.1:4317/api/pool", nil))
	token := initial.Header().Get("X-ASH-Session")
	req := httptest.NewRequest("POST", "http://127.0.0.1:4317/api/cleanup/bulk-plan", strings.NewReader(string(payload)))
	req.Header.Set("X-ASH-Session", token)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	if response.Code != http.StatusOK {
		t.Fatalf("large plan body rejected: %d %s", response.Code, response.Body.String())
	}
	var plans []CleanupPlan
	if err := json.Unmarshal(response.Body.Bytes(), &plans); err != nil {
		t.Fatal(err)
	}
	if len(plans) != len(ids) {
		t.Fatalf("got %d plans, want %d", len(plans), len(ids))
	}
}
