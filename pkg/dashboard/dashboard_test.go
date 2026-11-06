package dashboard

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imMamdouhaboammar/skill-healer/pkg/types"
)

func TestPoolEndpoint(t *testing.T) {
	pool := &types.Pool{
		Skills: map[string]types.Entity{"sample": {Name: "sample", Type: types.TypeSkill}},
		MCPs:   map[string]types.Entity{}, CLIs: map[string]types.Entity{}, Agents: map[string]types.Entity{},
		Total: 1,
	}
	request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:4317/api/pool", nil)
	response := httptest.NewRecorder()
	Handler(func() (*types.Pool, error) { return pool, nil }).ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.Code)
	}
	if response.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("inventory response must not be cached")
	}
	if response.Header().Get("Content-Security-Policy") == "" {
		t.Fatal("dashboard must set a CSP")
	}
	var decoded types.Pool
	if err := json.Unmarshal(response.Body.Bytes(), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Total != 1 || decoded.Skills["sample"].Name != "sample" {
		t.Fatalf("unexpected pool: %+v", decoded)
	}
}

// A mutation without a session token must be rejected before touching state.
func TestMutationRequiresSessionAndPersistsMetadata(t *testing.T) {
	home := t.TempDir()
	store := newStore(filepath.Join(home, ".ash", "db", "dashboard.json"), home)
	pool := &types.Pool{Skills: map[string]types.Entity{"sample": {ID: "skill:sample", Name: "sample", Type: types.TypeSkill}}, MCPs: map[string]types.Entity{}, CLIs: map[string]types.Entity{}, Agents: map[string]types.Entity{}}
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
	blocked := request(http.MethodPatch, "/api/tool", `{"id":"skill:sample","alias":"Pinned","favorite":true}`, "")
	if blocked.Code != http.StatusForbidden {
		t.Fatalf("missing token status = %d", blocked.Code)
	}
	view, _ := store.View()
	if len(view.Tools) != 0 {
		t.Fatal("rejected request wrote state")
	}
	initial := request(http.MethodGet, "/api/pool", "", "")
	token := initial.Header().Get("X-ASH-Session")
	if token == "" {
		t.Fatal("session token missing from pool response")
	}
	updated := request(http.MethodPatch, "/api/tool", `{"id":"skill:sample","alias":"Pinned","favorite":true,"tags":["writing"]}`, token)
	if updated.Code != http.StatusOK {
		t.Fatalf("update status = %d: %s", updated.Code, updated.Body.String())
	}
	view, _ = store.View()
	if view.Tools["skill:sample"].Alias != "Pinned" || !view.Tools["skill:sample"].Favorite {
		t.Fatal("metadata not saved")
	}
	unknown := request(http.MethodPatch, "/api/tool", `{"id":"missing","favorite":true}`, token)
	if unknown.Code != http.StatusNotFound {
		t.Fatalf("unknown item status %d", unknown.Code)
	}
	deleted := request(http.MethodDelete, "/api/tool", `{"id":"skill:sample"}`, token)
	if deleted.Code != http.StatusOK {
		t.Fatalf("delete metadata status %d", deleted.Code)
	}
	view, _ = store.View()
	if len(view.Tools) != 0 {
		t.Fatal("metadata not deleted")
	}
}

func TestRevealUsesDiscoveredPathAndRejectsUnknown(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, "KNOWN-SKILL.md")
	if err := os.WriteFile(path, []byte("# Known"), 0600); err != nil {
		t.Fatal(err)
	}
	pool := &types.Pool{Skills: map[string]types.Entity{"sample": {ID: "skill:sample", Name: "sample", Type: types.TypeSkill, Path: path}}, MCPs: map[string]types.Entity{}, CLIs: map[string]types.Entity{}, Agents: map[string]types.Entity{}}
	var revealed string
	handler := handlerWithStore(func() (*types.Pool, error) { return pool, nil }, newStore(filepath.Join(home, "dashboard.json"), home), func(path string) error { revealed = path; return nil })
	initial := httptest.NewRecorder()
	handler.ServeHTTP(initial, httptest.NewRequest("GET", "http://127.0.0.1:4317/api/pool", nil))
	token := initial.Header().Get("X-ASH-Session")
	r := httptest.NewRequest("POST", "http://127.0.0.1:4317/api/reveal", strings.NewReader(`{"id":"skill:sample"}`))
	r.Header.Set("X-ASH-Session", token)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != http.StatusOK || revealed != path {
		t.Fatalf("reveal result = %d path = %q", w.Code, revealed)
	}
	r = httptest.NewRequest("POST", "http://127.0.0.1:4317/api/reveal", strings.NewReader(`{"id":"not-present"}`))
	r.Header.Set("X-ASH-Session", token)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != http.StatusNotFound {
		t.Fatalf("unknown path should be rejected: %d", w.Code)
	}
}

func TestPreviewReadsOnlyDiscoveredSkillMarkdown(t *testing.T) {
	home := t.TempDir()
	skillPath := filepath.Join(home, "my-skill")
	if err := os.Mkdir(skillPath, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillPath, "SKILL.md"), []byte("# My skill\nInstructions"), 0600); err != nil {
		t.Fatal(err)
	}
	pool := &types.Pool{Skills: map[string]types.Entity{"sample": {ID: "skill:sample", Name: "sample", Type: types.TypeSkill, Path: skillPath}}, MCPs: map[string]types.Entity{}, CLIs: map[string]types.Entity{}, Agents: map[string]types.Entity{}}
	handler := handlerWithStore(func() (*types.Pool, error) { return pool, nil }, newStore(filepath.Join(home, "state.json"), home), func(string) error { return nil })
	request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:4317/api/preview?id=skill%3Asample", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, request)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "Instructions") {
		t.Fatalf("skill source not readable: %d %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "http://127.0.0.1:4317/api/preview?id=other", nil))
	if w.Code != http.StatusNotFound {
		t.Fatalf("unlisted source allowed: %d", w.Code)
	}
}

func TestMutationRejectsForeignOrigin(t *testing.T) {
	home := t.TempDir()
	store := newStore(filepath.Join(home, "state.json"), home)
	pool := &types.Pool{Skills: map[string]types.Entity{}, MCPs: map[string]types.Entity{}, CLIs: map[string]types.Entity{}, Agents: map[string]types.Entity{}}
	handler := handlerWithStore(func() (*types.Pool, error) { return pool, nil }, store, func(string) error { return nil })
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest("GET", "http://127.0.0.1:4317/api/pool", nil))
	token := w.Header().Get("X-ASH-Session")
	r := httptest.NewRequest("POST", "http://127.0.0.1:4317/api/shortcut", strings.NewReader(`{"label":"Bad origin","command":"whoami"}`))
	r.Header.Set("X-ASH-Session", token)
	r.Header.Set("Origin", "https://evil.example")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != http.StatusForbidden {
		t.Fatalf("foreign origin status = %d", w.Code)
	}
	view, _ := store.View()
	if len(view.Shortcuts) != 0 {
		t.Fatal("foreign site wrote a shortcut")
	}
}

func TestShortcutAndCollectionAPIEndToEnd(t *testing.T) {
	home := t.TempDir()
	store := newStore(filepath.Join(home, "state.json"), home)
	pool := &types.Pool{Skills: map[string]types.Entity{}, MCPs: map[string]types.Entity{}, CLIs: map[string]types.Entity{}, Agents: map[string]types.Entity{}}
	handler := handlerWithStore(func() (*types.Pool, error) { return pool, nil }, store, func(string) error { return nil })
	call := func(method, path, body, token string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "http://127.0.0.1:4317"+path, strings.NewReader(body))
		if token != "" {
			r.Header.Set("X-ASH-Session", token)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	token := call("GET", "/api/pool", "", "").Header().Get("X-ASH-Session")
	created := call("POST", "/api/shortcut", `{"label":"Stats","command":"ash stats"}`, token)
	if created.Code != 200 {
		t.Fatalf("shortcut creation failed %d %s", created.Code, created.Body.String())
	}
	var shortcut Shortcut
	if err := json.Unmarshal(created.Body.Bytes(), &shortcut); err != nil {
		t.Fatal(err)
	}
	if shortcut.ID == "" {
		t.Fatal("saved shortcut lacks ID")
	}
	updated := call("PUT", "/api/shortcut", `{"id":"`+shortcut.ID+`","label":"Tools","command":"ash pool --json"}`, token)
	if updated.Code != 200 {
		t.Fatalf("shortcut update failed: %d", updated.Code)
	}
	createdPreset := call("POST", "/api/preset", `{"name":"Research","tool_ids":["skill:writer"]}`, token)
	if createdPreset.Code != 200 {
		t.Fatalf("collection creation failed %d", createdPreset.Code)
	}
	var preset Preset
	if err := json.Unmarshal(createdPreset.Body.Bytes(), &preset); err != nil {
		t.Fatal(err)
	}
	view := call("GET", "/api/state", "", "")
	if !strings.Contains(view.Body.String(), "ash pool --json") || !strings.Contains(view.Body.String(), "Research") {
		t.Fatalf("state GET missing updates: %s", view.Body.String())
	}
	if deleted := call("DELETE", "/api/shortcut", `{"id":"`+shortcut.ID+`"}`, token); deleted.Code != 200 {
		t.Fatalf("shortcut delete failed: %d", deleted.Code)
	}
	if deleted := call("DELETE", "/api/preset", `{"id":"`+preset.ID+`"}`, token); deleted.Code != 200 {
		t.Fatalf("collection delete failed: %d", deleted.Code)
	}
	saved, _ := store.View()
	if len(saved.Shortcuts) != 0 || len(saved.Presets) != 0 {
		t.Fatal("CRUD records remain after delete")
	}
}

func TestStandbyAndRestoreThroughGuardedAPI(t *testing.T) {
	home := t.TempDir()
	active := filepath.Join(home, ".gemini", "config", "skills")
	if err := os.MkdirAll(active, 0700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(home, "source")
	if err := os.Mkdir(target, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(active, "writer")
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
	pool := &types.Pool{Skills: map[string]types.Entity{"writer": {ID: "skill:writer", Name: "writer", Type: types.TypeSkill, Path: path, IsSymlink: true}}}
	store := newStore(filepath.Join(home, ".ash", "db", "dashboard.json"), home)
	handler := handlerWithStore(func() (*types.Pool, error) { return pool, nil }, store, func(string) error { return nil })
	request := func(path string, token string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "http://127.0.0.1:4317"+path, strings.NewReader(`{"id":"skill:writer"}`))
		r.Header.Set("X-ASH-Session", token)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	initial := httptest.NewRecorder()
	handler.ServeHTTP(initial, httptest.NewRequest("GET", "http://127.0.0.1:4317/api/pool", nil))
	token := initial.Header().Get("X-ASH-Session")
	if w := request("/api/standby", token); w.Code != 200 {
		t.Fatalf("standby status %d", w.Code)
	}
	if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("active symlink still present")
	}
	if _, err := os.Stat(target); err != nil {
		t.Fatal("canonical skill removed")
	}
	if w := request("/api/restore", token); w.Code != 200 {
		t.Fatalf("restore status %d", w.Code)
	}
	if info, err := os.Lstat(path); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("restored symlink missing: %v", err)
	}
}

func TestCleanupAPIActuallyMovesFileAndRestoresWithSession(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, ".agents", "skills", "temporary")
	if err := os.MkdirAll(path, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "SKILL.md"), []byte("# Temporary"), 0600); err != nil {
		t.Fatal(err)
	}
	pool := &types.Pool{Skills: map[string]types.Entity{"temporary": {ID: "skill:temporary", Name: "temporary", Type: types.TypeSkill, Path: path}}}
	store := newStore(filepath.Join(home, ".ash", "db", "dashboard.json"), home)
	handler := handlerWithStore(func() (*types.Pool, error) { return pool, nil }, store, func(string) error { return nil })
	call := func(method, uri, body, token string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "http://127.0.0.1:4317"+uri, strings.NewReader(body))
		if token != "" {
			r.Header.Set("X-ASH-Session", token)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	token := call("GET", "/api/pool", "", "").Header().Get("X-ASH-Session")
	planResponse := call("GET", "/api/cleanup/plan?id=skill%3Atemporary", "", "")
	if planResponse.Code != 200 {
		t.Fatalf("plan status %d", planResponse.Code)
	}
	var plan CleanupPlan
	if err := json.Unmarshal(planResponse.Body.Bytes(), &plan); err != nil {
		t.Fatal(err)
	}
	if !plan.Allowed {
		t.Fatalf("local skill must be removable: %+v", plan)
	}
	payload := `{"id":"skill:temporary","fingerprint":"` + plan.Fingerprint + `","confirmation":"temporary"}`
	blocked := call("POST", "/api/cleanup/remove", payload, "")
	if blocked.Code != http.StatusForbidden {
		t.Fatalf("unguarded remove returned %d", blocked.Code)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal("unauthorized cleanup moved source")
	}
	removed := call("POST", "/api/cleanup/remove", payload, token)
	if removed.Code != 200 {
		t.Fatalf("remove status %d: %s", removed.Code, removed.Body.String())
	}
	var record TrashRecord
	if err := json.Unmarshal(removed.Body.Bytes(), &record); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("API cleanup left source on disk")
	}
	if call("POST", "/api/cleanup/restore", `{"id":"`+record.ID+`"}`, token).Code != 200 {
		t.Fatal("restore endpoint failed")
	}
	if _, err := os.Stat(filepath.Join(path, "SKILL.md")); err != nil {
		t.Fatal("API restoration failed")
	}
}

func TestPoolFailureDoesNotLeakErrors(t *testing.T) {
	response := httptest.NewRecorder()
	Handler(func() (*types.Pool, error) { return nil, errors.New("secret local path") }).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "http://127.0.0.1:4317/api/pool", nil))
	if response.Code != http.StatusServiceUnavailable || strings.Contains(response.Body.String(), "secret local path") {
		t.Fatalf("unsafe error response: %d %s", response.Code, response.Body.String())
	}
}

func TestDashboardServesHTMLAndBlocksAPIPost(t *testing.T) {
	handler := Handler(func() (*types.Pool, error) { return &types.Pool{}, nil })
	page := httptest.NewRecorder()
	handler.ServeHTTP(page, httptest.NewRequest(http.MethodGet, "http://127.0.0.1:4317/", nil))
	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), "Agentic Skills Healer") {
		t.Fatalf("dashboard missing: %d", page.Code)
	}
	post := httptest.NewRecorder()
	handler.ServeHTTP(post, httptest.NewRequest(http.MethodPost, "http://127.0.0.1:4317/api/pool", nil))
	if post.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST status = %d, want 405", post.Code)
	}
}

func TestForeignHostCannotReadLocalInventory(t *testing.T) {
	called := false
	handler := Handler(func() (*types.Pool, error) {
		called = true
		return &types.Pool{Skills: map[string]types.Entity{"secret": {Path: "/private/local/path"}}}, nil
	})
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest("GET", "http://attacker.invalid:4317/api/pool", nil))
	if w.Code != http.StatusForbidden || called || strings.Contains(w.Body.String(), "/private/local/path") {
		t.Fatalf("foreign Host may read inventory: code=%d, called=%v", w.Code, called)
	}
}
