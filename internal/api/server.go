package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/ThakerKush/Hatch/internal/config"
	"github.com/ThakerKush/Hatch/internal/store"
	"github.com/ThakerKush/Hatch/internal/vmm"
)

// Server exposes the Hatch REST API.
type Server struct {
	cfg config.Config
	db  *store.DB
	vmm *vmm.Manager
}

// NewServer creates an API server backed by the given database and VM manager.
func NewServer(cfg config.Config, db *store.DB, manager *vmm.Manager) *Server {
	return &Server{
		cfg: cfg,
		db:  db,
		vmm: manager,
	}
}

// Routes returns the HTTP handler with all API routes registered.
func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/healthz", s.handleHealth)

	// Images (temporarily hidden)
	// mux.HandleFunc("/images", s.handleImages)
	// mux.HandleFunc("/images/", s.handleImage)

	// VMs
	mux.HandleFunc("/vms", s.handleVMs)
	mux.HandleFunc("/vms/", s.handleVM)

	// Templates (temporarily hidden)
	// mux.HandleFunc("/templates", s.handleTemplates)
	// mux.HandleFunc("/templates/", s.handleTemplate)

	// Routes (top-level delete)
	mux.HandleFunc("/routes/", s.handleRouteDelete)

	return withLogging(withAPIKeyAuth(s.cfg, mux))
}

// ──────────────────────────── Health ────────────────────────────

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	vms, _ := s.db.ListVMs()
	routes, _ := s.db.ListAllRoutes()
	writeJSON(w, http.StatusOK, map[string]any{
		"status":      "ok",
		"vm_count":    len(vms),
		"route_count": len(routes),
	})
}

// ──────────────────────────── Images ────────────────────────────

func (s *Server) handleImages(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		var req createImageRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		if err := req.Validate(); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		image := req.ToImage(s.cfg)
		if err := s.db.CreateImage(image); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusCreated, image)
	case http.MethodGet:
		images, err := s.db.ListImages()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, images)
	default:
		writeError(w, http.StatusMethodNotAllowed, errMethodNotAllowed)
	}
}

func (s *Server) handleImage(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/images/")
	if id == "" {
		writeError(w, http.StatusBadRequest, errInvalidPath)
		return
	}

	switch r.Method {
	case http.MethodGet:
		image, err := s.db.GetImage(id)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		if image == nil {
			writeError(w, http.StatusNotFound, errNotFound)
			return
		}
		writeJSON(w, http.StatusOK, image)
	case http.MethodDelete:
		deleted, err := s.db.DeleteImage(id)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		if !deleted {
			writeError(w, http.StatusNotFound, errNotFound)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
	default:
		writeError(w, http.StatusMethodNotAllowed, errMethodNotAllowed)
	}
}

// ──────────────────────────── VMs ────────────────────────────

func (s *Server) handleVMs(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		var req createVMRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		if err := req.Validate(); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}

		// If a template_id is provided, resolve its defaults.
		if req.TemplateID != "" {
			tpl, err := s.db.GetTemplate(req.TemplateID)
			if err != nil {
				writeError(w, http.StatusInternalServerError, err)
				return
			}
			if tpl == nil {
				writeError(w, http.StatusBadRequest, errNotFound)
				return
			}
			// Template values are defaults; explicit request values override.
			if req.ImageID == "" {
				req.ImageID = tpl.ImageID
			}
			if req.VCPUCount == 0 {
				req.VCPUCount = tpl.VCPUCount
			}
			if req.MemMib == 0 {
				req.MemMib = tpl.MemMib
			}
			if req.UserData == "" {
				req.UserData = tpl.UserData
			}
		}

		// Resolve default image if image_id is still empty.
		if err := req.ResolveImageID(s.cfg); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}

		opts := req.ToOptions(s.cfg)
		vm, err := s.vmm.CreateAndStart(r.Context(), opts)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusCreated, vm)
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.vmm.List())
	default:
		writeError(w, http.StatusMethodNotAllowed, errMethodNotAllowed)
	}
}

func (s *Server) handleVM(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/vms/")
	parts := strings.Split(path, "/")
	if len(parts) == 0 || parts[0] == "" {
		writeError(w, http.StatusBadRequest, errInvalidPath)
		return
	}

	id := parts[0]

	// /vms/{id}
	if len(parts) == 1 {
		switch r.Method {
		case http.MethodGet:
			vm, ok := s.vmm.Get(id)
			if !ok {
				writeError(w, http.StatusNotFound, errNotFound)
				return
			}
			writeJSON(w, http.StatusOK, vm)
		case http.MethodDelete:
			if err := s.vmm.Delete(r.Context(), id); err != nil {
				writeError(w, http.StatusNotFound, err)
				return
			}
			writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
		default:
			writeError(w, http.StatusMethodNotAllowed, errMethodNotAllowed)
		}
		return
	}

	action := parts[1]
	switch {
	// POST /vms/{id}/stop
	case r.Method == http.MethodPost && action == "stop":
		vm, err := s.vmm.Stop(r.Context(), id)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, vm)

	// POST /vms/{id}/snapshot
	case r.Method == http.MethodPost && action == "snapshot":
		snap, err := s.vmm.Snapshot(r.Context(), id)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusCreated, snap)

	// POST /vms/{id}/restore
	case r.Method == http.MethodPost && action == "restore":
		vm, err := s.vmm.Restore(r.Context(), id)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, vm)

	// GET /vms/{id}/snapshots
	case r.Method == http.MethodGet && action == "snapshots":
		snaps, err := s.db.ListSnapshots(id)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, snaps)

	// POST /vms/{id}/routes
	case r.Method == http.MethodPost && action == "routes":
		var req createRouteRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		if err := req.Validate(); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		if _, ok := s.vmm.Get(id); !ok {
			writeError(w, http.StatusNotFound, errNotFound)
			return
		}
		if existing, _ := s.db.GetRouteBySubdomain(req.Subdomain); existing != nil {
			writeError(w, http.StatusConflict, fmt.Errorf("subdomain %q is already in use", req.Subdomain))
			return
		}
		route := req.ToRoute(id)
		if err := s.db.CreateRoute(route); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusCreated, route)

	// GET /vms/{id}/routes
	case r.Method == http.MethodGet && action == "routes":
		routes, err := s.db.ListRoutesByVM(id)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, routes)

	default:
		writeError(w, http.StatusNotFound, errNotFound)
	}
}

// ──────────────────────────── Templates ────────────────────────────

func (s *Server) handleTemplates(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		var req createTemplateRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		if err := req.Validate(); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		// Resolve default image if image_id is empty.
		if err := req.ResolveImageID(s.cfg); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		// Verify the image exists.
		img, err := s.db.GetImage(req.ImageID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		if img == nil {
			writeError(w, http.StatusBadRequest, errNotFound)
			return
		}
		tpl := req.ToTemplate(s.cfg)
		if err := s.db.CreateTemplate(tpl); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusCreated, tpl)
	case http.MethodGet:
		templates, err := s.db.ListTemplates()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, templates)
	default:
		writeError(w, http.StatusMethodNotAllowed, errMethodNotAllowed)
	}
}

func (s *Server) handleTemplate(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/templates/")
	if id == "" {
		writeError(w, http.StatusBadRequest, errInvalidPath)
		return
	}

	switch r.Method {
	case http.MethodGet:
		tpl, err := s.db.GetTemplate(id)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		if tpl == nil {
			writeError(w, http.StatusNotFound, errNotFound)
			return
		}
		writeJSON(w, http.StatusOK, tpl)
	case http.MethodDelete:
		deleted, err := s.db.DeleteTemplate(id)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		if !deleted {
			writeError(w, http.StatusNotFound, errNotFound)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
	default:
		writeError(w, http.StatusMethodNotAllowed, errMethodNotAllowed)
	}
}

// ──────────────────────────── Routes (top-level delete) ────────────────────────

func (s *Server) handleRouteDelete(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/routes/")
	if id == "" {
		writeError(w, http.StatusBadRequest, errInvalidPath)
		return
	}

	if r.Method != http.MethodDelete {
		writeError(w, http.StatusMethodNotAllowed, errMethodNotAllowed)
		return
	}

	deleted, err := s.db.DeleteRoute(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if !deleted {
		writeError(w, http.StatusNotFound, errNotFound)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// ──────────────────────────── Helpers ────────────────────────────

func withLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		slog.Info("http request", "method", r.Method, "path", r.URL.Path, "duration", time.Since(start).String())
	})
}

func withAPIKeyAuth(cfg config.Config, next http.Handler) http.Handler {
	verifyClient := &http.Client{Timeout: 5 * time.Second}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rawKey, err := extractAPIKey(r)
		if err != nil {
			writeError(w, http.StatusUnauthorized, err)
			return
		}

		valid, err := verifyAPIKey(verifyClient, cfg.BetterAuthVerifyURL, rawKey)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		if !valid {
			writeError(w, http.StatusUnauthorized, errors.New("invalid api key"))
			return
		}

		next.ServeHTTP(w, r)
	})
}

func verifyAPIKey(client *http.Client, endpoint string, key string) (bool, error) {
	body, _ := json.Marshal(map[string]string{"key": key})
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return false, fmt.Errorf("build verify request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return false, fmt.Errorf("verify api key: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false, nil
	}

	var payload struct {
		Valid bool `json:"valid"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return false, fmt.Errorf("decode verify response: %w", err)
	}
	return payload.Valid, nil
}

func extractAPIKey(r *http.Request) (string, error) {
	if bearer := strings.TrimSpace(r.Header.Get("Authorization")); bearer != "" {
		const prefix = "Bearer "
		if !strings.HasPrefix(bearer, prefix) || strings.TrimSpace(strings.TrimPrefix(bearer, prefix)) == "" {
			return "", errors.New("invalid authorization header")
		}
		return strings.TrimSpace(strings.TrimPrefix(bearer, prefix)), nil
	}

	if key := strings.TrimSpace(r.Header.Get("X-API-Key")); key != "" {
		return key, nil
	}
	return "", errors.New("missing api key")
}

func decodeJSON(r *http.Request, out any) error {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	return decoder.Decode(out)
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}
