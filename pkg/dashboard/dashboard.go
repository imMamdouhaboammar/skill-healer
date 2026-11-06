package dashboard

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"github.com/imMamdouhaboammar/skill-healer/pkg/scanner"
	"github.com/imMamdouhaboammar/skill-healer/pkg/telemetry"
	"github.com/imMamdouhaboammar/skill-healer/pkg/types"
)

//go:embed static/*
var files embed.FS

type SnapshotFunc func() (*types.Pool, error)

func liveSnapshot() (*types.Pool, error) {
	db, err := telemetry.Load("")
	if err != nil {
		return nil, err
	}
	pool, err := scanner.ScanPool(db)
	if err != nil {
		return nil, err
	}
	for _, group := range []struct {
		entities map[string]types.Entity
		kind     types.EntityType
	}{
		{pool.MCPs, types.TypeMCP},
		{pool.CLIs, types.TypeCLI},
		{pool.Agents, types.TypeAgent},
	} {
		for name, item := range group.entities {
			record, exists := telemetry.GetRecord(db, group.kind, name)
			if !exists {
				continue
			}
			item.CallCount = record.CallCount
			item.VerifiedCalls = record.VerifiedCalls
			item.Tier = types.Tier(record.Tier)
			if record.FirstSeen > 0 {
				t := time.Unix(int64(record.FirstSeen), 0)
				item.FirstSeen = &t
			}
			if record.LastSeen > 0 {
				t := time.Unix(int64(record.LastSeen), 0)
				item.LastSeen = &t
			}
			group.entities[name] = item
		}
	}
	return pool, nil
}

// Handler serves the local dashboard and its guarded preference endpoints.
func Handler(snapshot SnapshotFunc) http.Handler {
	store, err := defaultStore()
	if err != nil {
		panic(err)
	}
	return handlerWithStore(snapshot, store, func(path string) error { return exec.Command("open", "-R", path).Run() })
}

func handlerWithStore(snapshot SnapshotFunc, store *Store, reveal func(string) error) http.Handler {
	assets, err := fs.Sub(files, "static")
	if err != nil {
		panic(err)
	}

	mux := http.NewServeMux()
	session, err := randomID()
	if err != nil {
		panic(err)
	}
	mux.HandleFunc("GET /api/pool", func(w http.ResponseWriter, r *http.Request) {
		pool, err := snapshot()
		if err != nil {
			http.Error(w, "Inventory scan failed", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-ASH-Session", session)
		if err := json.NewEncoder(w).Encode(pool); err != nil {
			return
		}
	})
	registerAPI(mux, snapshot, store, reveal, session)
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/pool" || r.URL.Path == "/api/state" || r.URL.Path == "/api/preview" || r.URL.Path == "/api/tool" || r.URL.Path == "/api/shortcut" || r.URL.Path == "/api/preset" || r.URL.Path == "/api/reveal" || r.URL.Path == "/api/standby" || r.URL.Path == "/api/restore" || strings.HasPrefix(r.URL.Path, "/api/cleanup/") {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		http.NotFound(w, r)
	})
	mux.Handle("/", http.FileServer(http.FS(assets)))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'self'; img-src 'self' data:; connect-src 'self'; font-src 'self'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'")
		if !localHost(r.Host) {
			http.Error(w, "Local host required", http.StatusForbidden)
			return
		}
		mux.ServeHTTP(w, r)
	})
}

// Serve listens only on loopback to avoid exposing local tool paths on the network.
func Serve(port int) error {
	if port < 1 || port > 65535 {
		return errors.New("port must be between 1 and 65535")
	}
	address := fmt.Sprintf("127.0.0.1:%d", port)
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return fmt.Errorf("dashboard listener: %w", err)
	}
	fmt.Printf("Agentic Skills Healer dashboard: http://%s\n", address)
	server := &http.Server{
		Handler:           Handler(liveSnapshot),
		ReadHeaderTimeout: 5 * time.Second,
		// Large, explicitly confirmed local cleanups can outlast 30 seconds.
		WriteTimeout: 0,
		IdleTimeout:  90 * time.Second,
	}
	return server.Serve(listener)
}
