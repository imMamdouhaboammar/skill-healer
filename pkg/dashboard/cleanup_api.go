package dashboard

import (
	"net/http"
)

type cleanupRequest struct {
	ID           string `json:"id"`
	Fingerprint  string `json:"fingerprint"`
	Confirmation string `json:"confirmation"`
}

func registerCleanupAPI(mux *http.ServeMux, snapshot SnapshotFunc, store *Store, session string) {
	manager := newCleanupManager(store)
	registerBulkAPI(mux, snapshot, manager, store, session)
	mux.HandleFunc("GET /api/cleanup/plan", func(w http.ResponseWriter, r *http.Request) {
		item, err := findTool(snapshot, r.URL.Query().Get("id"))
		if err != nil {
			apiError(w, err)
			return
		}
		plan, err := manager.Plan(item)
		if err != nil {
			apiError(w, err)
			return
		}
		jsonResponse(w, plan)
	})
	mux.HandleFunc("POST /api/cleanup/remove", guardMutation(func(w http.ResponseWriter, r *http.Request) {
		var req cleanupRequest
		if err := readPayload(r, &req); err != nil {
			apiError(w, err)
			return
		}
		item, err := findTool(snapshot, req.ID)
		if err != nil {
			apiError(w, err)
			return
		}
		record, err := manager.Remove(item, req.Fingerprint, req.Confirmation)
		if err != nil {
			apiError(w, err)
			return
		}
		jsonResponse(w, record)
	}, session))
	mux.HandleFunc("POST /api/cleanup/restore", guardMutation(func(w http.ResponseWriter, r *http.Request) {
		var req idPayload
		if err := readPayload(r, &req); err != nil {
			apiError(w, err)
			return
		}
		if err := manager.Restore(req.ID); err != nil {
			apiError(w, err)
			return
		}
		jsonResponse(w, map[string]bool{"ok": true})
	}, session))
	mux.HandleFunc("POST /api/cleanup/purge", guardMutation(func(w http.ResponseWriter, r *http.Request) {
		var req cleanupRequest
		if err := readPayload(r, &req); err != nil {
			apiError(w, err)
			return
		}
		if err := manager.Purge(req.ID, req.Confirmation); err != nil {
			apiError(w, err)
			return
		}
		jsonResponse(w, map[string]bool{"ok": true})
	}, session))
}
