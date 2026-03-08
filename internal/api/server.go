package api

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/ThakerKush/Hatch/internal/config"
	"github.com/ThakerKush/Hatch/internal/store"
	"github.com/ThakerKush/Hatch/internal/vmm"
	"golang.org/x/crypto/ssh"
)

// Server exposes the Hatch REST API.
type Server struct {
	cfg config.Config
	db  *store.DB
	vmm *vmm.Manager
}

type contextKey string

const userIDContextKey contextKey = "hatch_user_id"

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

	return withLogging(withAuth(s.cfg, mux))
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
	userID, ok := userIDFromCtx(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, errors.New("missing user in auth context"))
		return
	}

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

		count, err := s.db.CountVMsByUser(userID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		if count >= s.cfg.MaxVMsPerUser {
			writeError(
				w,
				http.StatusForbidden,
				fmt.Errorf("vm limit reached (%d/%d)", count, s.cfg.MaxVMsPerUser),
			)
			return
		}

		opts := req.ToOptions(s.cfg)
		opts.UserID = userID
		vm, err := s.vmm.CreateAndStart(r.Context(), opts)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusCreated, vm)
	case http.MethodGet:
		vms, err := s.db.ListVMsByUser(userID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		ptrs := make([]*store.VM, len(vms))
		for i := range vms {
			ptrs[i] = &vms[i]
		}
		writeJSON(w, http.StatusOK, ptrs)
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
			vm, err := s.getOwnedVM(r.Context(), id)
			if err != nil {
				writeError(w, http.StatusInternalServerError, err)
				return
			}
			if vm == nil {
				writeError(w, http.StatusNotFound, errNotFound)
				return
			}
			writeJSON(w, http.StatusOK, vm)
		case http.MethodDelete:
			vm, err := s.getOwnedVM(r.Context(), id)
			if err != nil {
				writeError(w, http.StatusInternalServerError, err)
				return
			}
			if vm == nil {
				writeError(w, http.StatusNotFound, errNotFound)
				return
			}
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
		vm, err := s.getOwnedVM(r.Context(), id)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		if vm == nil {
			writeError(w, http.StatusNotFound, errNotFound)
			return
		}
		vm, err = s.vmm.Stop(r.Context(), id)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, vm)

	// POST /vms/{id}/snapshot
	case r.Method == http.MethodPost && action == "snapshot":
		vm, err := s.getOwnedVM(r.Context(), id)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		if vm == nil {
			writeError(w, http.StatusNotFound, errNotFound)
			return
		}
		snaps, err := s.db.ListSnapshots(vm.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		if len(snaps) >= s.cfg.MaxSnapshotsPerVM {
			writeError(
				w,
				http.StatusForbidden,
				fmt.Errorf("snapshot limit reached (%d/%d)", len(snaps), s.cfg.MaxSnapshotsPerVM),
			)
			return
		}
		snap, err := s.vmm.Snapshot(r.Context(), id)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusCreated, snap)

	// POST /vms/{id}/restore
	case r.Method == http.MethodPost && action == "restore":
		vm, err := s.getOwnedVM(r.Context(), id)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		if vm == nil {
			writeError(w, http.StatusNotFound, errNotFound)
			return
		}
		vm, err = s.vmm.Restore(r.Context(), id)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, vm)

	// GET /vms/{id}/snapshots
	case r.Method == http.MethodGet && action == "snapshots":
		vm, err := s.getOwnedVM(r.Context(), id)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		if vm == nil {
			writeError(w, http.StatusNotFound, errNotFound)
			return
		}
		snaps, err := s.db.ListSnapshots(id)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, snaps)

	// GET /vms/{id}/metrics
	case r.Method == http.MethodGet && action == "metrics":
		vm, err := s.getOwnedVM(r.Context(), id)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		if vm == nil {
			writeError(w, http.StatusNotFound, errNotFound)
			return
		}
		metrics, err := readLatestVMMetrics(*vm)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, metrics)

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
		vm, err := s.getOwnedVM(r.Context(), id)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		if vm == nil {
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
		vm, err := s.getOwnedVM(r.Context(), id)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		if vm == nil {
			writeError(w, http.StatusNotFound, errNotFound)
			return
		}
		routes, err := s.db.ListRoutesByVM(id)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, routes)

	// POST /vms/{id}/connect
	case r.Method == http.MethodPost && action == "connect":
		s.handleConnect(w, r, id)

	// POST /vms/{id}/exec
	case r.Method == http.MethodPost && action == "exec":
		s.handleExec(w, r, id)

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

// ──────────────────────────── Connect / Exec ────────────────────────────

func (s *Server) handleConnect(w http.ResponseWriter, r *http.Request, vmID string) {
	if !s.vmm.SSHCAEnabled() {
		writeError(w, http.StatusServiceUnavailable, errors.New("SSH CA is not configured"))
		return
	}

	vm, err := s.getOwnedVM(r.Context(), vmID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if vm == nil {
		writeError(w, http.StatusNotFound, errNotFound)
		return
	}
	if vm.State != store.VMStateRunning {
		writeError(w, http.StatusConflict, fmt.Errorf("vm is %s, must be running", vm.State))
		return
	}
	if vm.SSHPort <= 0 {
		writeError(w, http.StatusConflict, errors.New("vm has no SSH port (network not enabled)"))
		return
	}

	var req connectRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if strings.TrimSpace(req.PublicKey) == "" {
		writeError(w, http.StatusBadRequest, errors.New("public_key is required"))
		return
	}

	pubKey, _, _, _, err := ssh.ParseAuthorizedKey([]byte(req.PublicKey))
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("invalid public key: %w", err))
		return
	}

	cert, err := s.vmm.SignSSHCert(pubKey, vm.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("sign certificate: %w", err))
		return
	}

	host := s.cfg.HTTPAddr
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}

	writeJSON(w, http.StatusOK, connectResponse{
		Certificate: string(ssh.MarshalAuthorizedKey(cert)),
		Host:        host,
		Port:        vm.SSHPort,
		Username:    "agent",
		ValidUntil:  int64(cert.ValidBefore),
	})
}

func (s *Server) handleExec(w http.ResponseWriter, r *http.Request, vmID string) {
	if !s.vmm.SSHCAEnabled() {
		writeError(w, http.StatusServiceUnavailable, errors.New("SSH CA is not configured"))
		return
	}

	vm, err := s.getOwnedVM(r.Context(), vmID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if vm == nil {
		writeError(w, http.StatusNotFound, errNotFound)
		return
	}
	if vm.State != store.VMStateRunning {
		writeError(w, http.StatusConflict, fmt.Errorf("vm is %s, must be running", vm.State))
		return
	}
	if vm.GuestIP == "" {
		writeError(w, http.StatusConflict, errors.New("vm has no guest IP (network not enabled)"))
		return
	}

	var req execRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if strings.TrimSpace(req.Command) == "" {
		writeError(w, http.StatusBadRequest, errors.New("command is required"))
		return
	}

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("generate ephemeral key: %w", err))
		return
	}

	sshPub, err := ssh.NewPublicKey(pub)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("convert public key: %w", err))
		return
	}

	cert, err := s.vmm.SignSSHCert(sshPub, vm.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("sign certificate: %w", err))
		return
	}

	signer, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("create signer: %w", err))
		return
	}
	certSigner, err := ssh.NewCertSigner(cert, signer)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("create cert signer: %w", err))
		return
	}

	clientCfg := &ssh.ClientConfig{
		User:            "agent",
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(certSigner)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	}

	addr := net.JoinHostPort(vm.GuestIP, "22")
	client, err := ssh.Dial("tcp", addr, clientCfg)
	if err != nil {
		writeError(w, http.StatusBadGateway, fmt.Errorf("ssh connect to vm: %w", err))
		return
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		writeError(w, http.StatusBadGateway, fmt.Errorf("ssh session: %w", err))
		return
	}
	defer session.Close()

	var stdout, stderr bytes.Buffer
	session.Stdout = &stdout
	session.Stderr = &stderr

	exitCode := 0
	if err := session.Run(req.Command); err != nil {
		if exitErr, ok := err.(*ssh.ExitError); ok {
			exitCode = exitErr.ExitStatus()
		} else {
			writeError(w, http.StatusBadGateway, fmt.Errorf("exec command: %w", err))
			return
		}
	}

	writeJSON(w, http.StatusOK, execResponse{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		ExitCode: exitCode,
	})
}

// ──────────────────────────── Helpers ────────────────────────────

func withLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		slog.Info("http request", "method", r.Method, "path", r.URL.Path, "duration", time.Since(start).String())
	})
}

func withAuth(cfg config.Config, next http.Handler) http.Handler {
	authClient := &http.Client{Timeout: 5 * time.Second}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Path 1: API key (Authorization: Bearer / X-API-Key)
		if rawKey, err := extractAPIKey(r); err == nil {
			result, err := verifyAPIKey(authClient, cfg.BetterAuthVerifyURL, rawKey)
			if err != nil {
				writeError(w, http.StatusInternalServerError, err)
				return
			}
			if result.Valid && strings.TrimSpace(result.UserID) != "" {
				ctx := context.WithValue(r.Context(), userIDContextKey, result.UserID)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}
			writeError(w, http.StatusUnauthorized, errors.New("invalid api key"))
			return
		}

		// Path 2: session cookie (forwarded to Better Auth get-session)
		userID, err := verifySession(authClient, cfg.BetterAuthSessionURL, r)
		if err != nil {
			slog.Warn("session verification failed", "error", err)
			writeError(w, http.StatusUnauthorized, errors.New("unauthorized"))
			return
		}
		if strings.TrimSpace(userID) == "" {
			writeError(w, http.StatusUnauthorized, errors.New("unauthorized"))
			return
		}

		ctx := context.WithValue(r.Context(), userIDContextKey, userID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func verifySession(client *http.Client, endpoint string, original *http.Request) (string, error) {
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return "", fmt.Errorf("build session request: %w", err)
	}

	for _, cookie := range original.Cookies() {
		req.AddCookie(cookie)
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("verify session: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", nil
	}

	var payload struct {
		Session *struct {
			UserID string `json:"userId"`
		} `json:"session"`
		User *struct {
			ID string `json:"id"`
		} `json:"user"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", fmt.Errorf("decode session response: %w", err)
	}

	if payload.User != nil && strings.TrimSpace(payload.User.ID) != "" {
		return strings.TrimSpace(payload.User.ID), nil
	}
	if payload.Session != nil && strings.TrimSpace(payload.Session.UserID) != "" {
		return strings.TrimSpace(payload.Session.UserID), nil
	}
	return "", nil
}

type apiKeyVerifyResult struct {
	Valid  bool
	UserID string
}

func verifyAPIKey(client *http.Client, endpoint string, key string) (apiKeyVerifyResult, error) {
	body, _ := json.Marshal(map[string]string{"key": key})
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return apiKeyVerifyResult{}, fmt.Errorf("build verify request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return apiKeyVerifyResult{}, fmt.Errorf("verify api key: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return apiKeyVerifyResult{Valid: false}, nil
	}

	var payload struct {
		Valid  bool   `json:"valid"`
		UserID string `json:"userId"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return apiKeyVerifyResult{}, fmt.Errorf("decode verify response: %w", err)
	}
	return apiKeyVerifyResult{
		Valid:  payload.Valid,
		UserID: strings.TrimSpace(payload.UserID),
	}, nil
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

func userIDFromCtx(ctx context.Context) (string, bool) {
	userID, ok := ctx.Value(userIDContextKey).(string)
	if !ok || strings.TrimSpace(userID) == "" {
		return "", false
	}
	return strings.TrimSpace(userID), true
}

func (s *Server) getOwnedVM(ctx context.Context, vmID string) (*store.VM, error) {
	userID, ok := userIDFromCtx(ctx)
	if !ok {
		return nil, nil
	}

	vm, err := s.db.GetVM(vmID)
	if err != nil {
		return nil, err
	}
	if vm == nil || vm.UserID != userID {
		return nil, nil
	}
	return vm, nil
}
