package dashboard

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/imMamdouhaboammar/skill-healer/pkg/types"
)

type itemPayload struct {
	ID       string   `json:"id"`
	Alias    string   `json:"alias"`
	Domain   string   `json:"domain"`
	Notes    string   `json:"notes"`
	Tags     []string `json:"tags"`
	Favorite bool     `json:"favorite"`
	Hidden   bool     `json:"hidden"`
}

type idPayload struct {
	ID string `json:"id"`
}

func findTool(snapshot SnapshotFunc, id string) (types.Entity, error) {
	if id == "" {
		return types.Entity{}, ErrNotFound
	}
	pool, err := snapshot()
	if err != nil {
		return types.Entity{}, err
	}
	for _, group := range []map[string]types.Entity{pool.Skills, pool.MCPs, pool.CLIs, pool.Agents} {
		for _, item := range group {
			if item.ID == id {
				return item, nil
			}
		}
	}
	return types.Entity{}, ErrNotFound
}

func readPayload(r *http.Request, dest any) error {
	if r.ContentLength > 8192 {
		return errors.New("request too large")
	}
	decoder := json.NewDecoder(io.LimitReader(r.Body, 8193))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dest); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return errors.New("unexpected trailing data")
	}
	return nil
}

func jsonResponse(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(value)
}

func apiError(w http.ResponseWriter, err error) {
	status := http.StatusBadRequest
	switch {
	case errors.Is(err, ErrNotFound), errors.Is(err, os.ErrNotExist):
		status = http.StatusNotFound
	case errors.Is(err, ErrConflict):
		status = http.StatusConflict
	case errors.Is(err, ErrUnsafe):
		status = http.StatusForbidden
	case errors.Is(err, os.ErrPermission):
		status = http.StatusForbidden
	}
	message := "Request cannot be completed"
	if status == http.StatusNotFound {
		message = "Tool or record not found"
	}
	if status == http.StatusConflict {
		message = "Source changed or destination already exists. Refresh and try again"
	}
	if status == http.StatusForbidden {
		message = "This operation is not allowed for this tool"
	}
	if errors.Is(err, os.ErrPermission) {
		message = "macOS permissions denied access to this source"
	}
	http.Error(w, message, status)
}

func localMutation(r *http.Request, token string) bool {
	if r.Header.Get("X-ASH-Session") != token {
		return false
	}
	host := r.Host
	if !localHost(host) {
		return false
	}
	if origin := r.Header.Get("Origin"); origin != "" && origin != "http://"+host {
		return false
	}
	return true
}

func localHost(host string) bool {
	host = strings.ToLower(host)
	return host == "localhost" || host == "127.0.0.1" || strings.HasPrefix(host, "localhost:") || strings.HasPrefix(host, "127.0.0.1:")
}

func guardMutation(handler http.HandlerFunc, session string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !localMutation(r, session) {
			http.Error(w, "Local session required", http.StatusForbidden)
			return
		}
		handler(w, r)
	}
}

func registerAPI(mux *http.ServeMux, snapshot SnapshotFunc, store *Store, reveal func(string) error, session string) {
	registerCleanupAPI(mux, snapshot, store, session)
	mux.HandleFunc("GET /api/state", func(w http.ResponseWriter, r *http.Request) {
		view, err := store.View()
		if err != nil {
			http.Error(w, "Cannot read local preferences", http.StatusServiceUnavailable)
			return
		}
		jsonResponse(w, view)
	})
	mux.HandleFunc("GET /api/preview", func(w http.ResponseWriter, r *http.Request) {
		item, err := findTool(snapshot, r.URL.Query().Get("id"))
		if err != nil {
			apiError(w, err)
			return
		}
		result, err := skillPreview(item)
		if err != nil {
			apiError(w, err)
			return
		}
		jsonResponse(w, result)
	})
	mux.HandleFunc("PATCH /api/tool", guardMutation(func(w http.ResponseWriter, r *http.Request) {
		var req itemPayload
		if err := readPayload(r, &req); err != nil {
			apiError(w, err)
			return
		}
		if _, err := findTool(snapshot, req.ID); err != nil {
			apiError(w, err)
			return
		}
		pref := ToolPreference{Alias: req.Alias, Domain: types.Domain(req.Domain), Notes: req.Notes, Tags: req.Tags, Favorite: req.Favorite, Hidden: req.Hidden}
		if err := store.SetTool(req.ID, pref); err != nil {
			apiError(w, err)
			return
		}
		jsonResponse(w, map[string]bool{"ok": true})
	}, session))
	mux.HandleFunc("DELETE /api/tool", guardMutation(func(w http.ResponseWriter, r *http.Request) {
		var req idPayload
		if err := readPayload(r, &req); err != nil {
			apiError(w, err)
			return
		}
		if req.ID == "" {
			apiError(w, ErrNotFound)
			return
		}
		if err := store.DeleteTool(req.ID); err != nil {
			apiError(w, err)
			return
		}
		jsonResponse(w, map[string]bool{"ok": true})
	}, session))
	mux.HandleFunc("POST /api/shortcut", guardMutation(func(w http.ResponseWriter, r *http.Request) {
		var shortcut Shortcut
		if err := readPayload(r, &shortcut); err != nil {
			apiError(w, err)
			return
		}
		shortcut.ID = ""
		saved, err := store.SaveShortcut(shortcut)
		if err != nil {
			apiError(w, err)
			return
		}
		jsonResponse(w, saved)
	}, session))
	mux.HandleFunc("PUT /api/shortcut", guardMutation(func(w http.ResponseWriter, r *http.Request) {
		var shortcut Shortcut
		if err := readPayload(r, &shortcut); err != nil {
			apiError(w, err)
			return
		}
		if shortcut.ID == "" {
			apiError(w, ErrNotFound)
			return
		}
		saved, err := store.SaveShortcut(shortcut)
		if err != nil {
			apiError(w, err)
			return
		}
		jsonResponse(w, saved)
	}, session))
	mux.HandleFunc("DELETE /api/shortcut", guardMutation(func(w http.ResponseWriter, r *http.Request) {
		var req idPayload
		if err := readPayload(r, &req); err != nil {
			apiError(w, err)
			return
		}
		if err := store.DeleteShortcut(req.ID); err != nil {
			apiError(w, err)
			return
		}
		jsonResponse(w, map[string]bool{"ok": true})
	}, session))
	mux.HandleFunc("POST /api/preset", guardMutation(func(w http.ResponseWriter, r *http.Request) {
		var preset Preset
		if err := readPayload(r, &preset); err != nil {
			apiError(w, err)
			return
		}
		preset.ID = ""
		saved, err := store.SavePreset(preset)
		if err != nil {
			apiError(w, err)
			return
		}
		jsonResponse(w, saved)
	}, session))
	mux.HandleFunc("PUT /api/preset", guardMutation(func(w http.ResponseWriter, r *http.Request) {
		var preset Preset
		if err := readPayload(r, &preset); err != nil {
			apiError(w, err)
			return
		}
		if preset.ID == "" {
			apiError(w, ErrNotFound)
			return
		}
		saved, err := store.SavePreset(preset)
		if err != nil {
			apiError(w, err)
			return
		}
		jsonResponse(w, saved)
	}, session))
	mux.HandleFunc("DELETE /api/preset", guardMutation(func(w http.ResponseWriter, r *http.Request) {
		var req idPayload
		if err := readPayload(r, &req); err != nil {
			apiError(w, err)
			return
		}
		if err := store.DeletePreset(req.ID); err != nil {
			apiError(w, err)
			return
		}
		jsonResponse(w, map[string]bool{"ok": true})
	}, session))
	mux.HandleFunc("POST /api/reveal", guardMutation(func(w http.ResponseWriter, r *http.Request) {
		var req idPayload
		if err := readPayload(r, &req); err != nil {
			apiError(w, err)
			return
		}
		item, err := findTool(snapshot, req.ID)
		if err != nil {
			apiError(w, err)
			return
		}
		if item.Path == "" {
			apiError(w, ErrNotFound)
			return
		}
		if _, err := os.Lstat(item.Path); err != nil {
			apiError(w, ErrNotFound)
			return
		}
		if err := reveal(item.Path); err != nil {
			http.Error(w, "Cannot open Finder", http.StatusServiceUnavailable)
			return
		}
		jsonResponse(w, map[string]bool{"ok": true})
	}, session))
	mux.HandleFunc("POST /api/standby", guardMutation(func(w http.ResponseWriter, r *http.Request) {
		var req idPayload
		if err := readPayload(r, &req); err != nil {
			apiError(w, err)
			return
		}
		item, err := findTool(snapshot, req.ID)
		if err != nil {
			apiError(w, err)
			return
		}
		if err := store.MoveToStandby(item); err != nil {
			apiError(w, err)
			return
		}
		jsonResponse(w, map[string]bool{"ok": true})
	}, session))
	mux.HandleFunc("POST /api/restore", guardMutation(func(w http.ResponseWriter, r *http.Request) {
		var req idPayload
		if err := readPayload(r, &req); err != nil {
			apiError(w, err)
			return
		}
		if err := store.RestoreFromStandby(req.ID); err != nil {
			apiError(w, err)
			return
		}
		jsonResponse(w, map[string]bool{"ok": true})
	}, session))
}

func skillPreview(item types.Entity) (map[string]any, error) {
	if item.Type != types.TypeSkill || item.Path == "" {
		return nil, ErrUnsafe
	}
	path := item.Path
	info, err := os.Stat(path)
	if err != nil {
		return nil, ErrNotFound
	}
	if info.IsDir() {
		for _, filename := range []string{"SKILL.md", "README.md"} {
			candidate := filepath.Join(path, filename)
			if _, err := os.Stat(candidate); err == nil {
				path = candidate
				break
			}
		}
	}
	if !strings.EqualFold(filepath.Ext(path), ".md") {
		return nil, ErrNotFound
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, ErrNotFound
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, 32769))
	if err != nil {
		return nil, err
	}
	truncated := len(data) > 32768
	if truncated {
		data = data[:32768]
	}
	return map[string]any{"file": filepath.Base(path), "content": string(data), "truncated": truncated}, nil
}
