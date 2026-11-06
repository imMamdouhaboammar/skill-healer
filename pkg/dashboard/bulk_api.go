package dashboard

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

// Bulk requests can describe every discovered tool. Keep a transport-size
// safeguard without imposing a count-based selection or removal limit.
func readBulkPayload(r *http.Request, destination any) error {
	const maxBytes = 64 << 20
	if r.ContentLength > maxBytes {
		return errors.New("bulk request body too large")
	}
	decoder := json.NewDecoder(io.LimitReader(r.Body, maxBytes+1))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return errors.New("unexpected trailing bulk data")
	}
	return nil
}

type bulkMetadataRequest struct {
	IDs    []string `json:"ids"`
	Action string   `json:"action"`
}

func registerBulkAPI(mux *http.ServeMux, snapshot SnapshotFunc, manager *cleanupManager, store *Store, session string) {
	mux.HandleFunc("POST /api/cleanup/bulk-plan", guardMutation(func(w http.ResponseWriter, r *http.Request) {
		var req BulkIDsRequest
		if err := readBulkPayload(r, &req); err != nil {
			apiError(w, err)
			return
		}
		pool, err := snapshot()
		if err != nil {
			apiError(w, err)
			return
		}
		plans, err := bulkPlans(pool, manager, req.IDs)
		if err != nil {
			apiError(w, err)
			return
		}
		jsonResponse(w, plans)
	}, session))
	mux.HandleFunc("POST /api/cleanup/bulk-remove", guardMutation(func(w http.ResponseWriter, r *http.Request) {
		var req BulkRemoveRequest
		if err := readBulkPayload(r, &req); err != nil {
			apiError(w, err)
			return
		}
		pool, err := snapshot()
		if err != nil {
			apiError(w, err)
			return
		}
		result, err := bulkRemove(pool, manager, req)
		if err != nil {
			apiError(w, err)
			return
		}
		jsonResponse(w, result)
	}, session))
	mux.HandleFunc("POST /api/cleanup/bulk-restore", guardMutation(func(w http.ResponseWriter, r *http.Request) {
		var req BulkIDsRequest
		if err := readBulkPayload(r, &req); err != nil {
			apiError(w, err)
			return
		}
		result, err := bulkTrash(manager, req.IDs, "restore", "")
		if err != nil {
			apiError(w, err)
			return
		}
		jsonResponse(w, result)
	}, session))
	mux.HandleFunc("POST /api/cleanup/bulk-purge", guardMutation(func(w http.ResponseWriter, r *http.Request) {
		var req BulkIDsRequest
		if err := readBulkPayload(r, &req); err != nil {
			apiError(w, err)
			return
		}
		result, err := bulkTrash(manager, req.IDs, "purge", req.Confirmation)
		if err != nil {
			apiError(w, err)
			return
		}
		jsonResponse(w, result)
	}, session))
	mux.HandleFunc("POST /api/tools/bulk", guardMutation(func(w http.ResponseWriter, r *http.Request) {
		var req bulkMetadataRequest
		if err := readBulkPayload(r, &req); err != nil {
			apiError(w, err)
			return
		}
		if err := validateBulkIDs(req.IDs); err != nil {
			apiError(w, err)
			return
		}
		pool, err := snapshot()
		if err != nil {
			apiError(w, err)
			return
		}
		for _, id := range req.IDs {
			if _, ok := lookupPool(pool, id); !ok {
				apiError(w, ErrNotFound)
				return
			}
		}
		if err := store.BulkUpdateTools(req.IDs, req.Action); err != nil {
			apiError(w, err)
			return
		}
		jsonResponse(w, map[string]any{"ok": true, "updated": len(req.IDs), "action": req.Action})
	}, session))
}
