package api

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/ThakerKush/Hatch/internal/config"
	"github.com/ThakerKush/Hatch/internal/store"
	"github.com/ThakerKush/Hatch/internal/vmm"
)

type Server struct {
	cfg    config.Config
	images *store.ImageStore
	vmm    *vmm.Manager
}

func NewServer(cfg config.Config, images *store.ImageStore, manager *vmm.Manager) *Server {
	return &Server{
		cfg:    cfg,
		images: images,
		vmm:    manager,
	}
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/healthz", s.handleHealth)
	mux.HandleFunc("/images", s.handleImages)
	mux.HandleFunc("/images/", s.handleImage)
	mux.HandleFunc("/vms", s.handleVMs)
	mux.HandleFunc("/vms/", s.handleVM)

	return withLogging(mux)
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

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
		if err := s.images.Add(image); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusCreated, image)
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.images.List())
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
		image, ok := s.images.Get(id)
		if !ok {
			writeError(w, http.StatusNotFound, errNotFound)
			return
		}
		writeJSON(w, http.StatusOK, image)
	case http.MethodDelete:
		if !s.images.Delete(id) {
			writeError(w, http.StatusNotFound, errNotFound)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
	default:
		writeError(w, http.StatusMethodNotAllowed, errMethodNotAllowed)
	}
}

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
	case r.Method == http.MethodPost && action == "stop":
		vm, err := s.vmm.Stop(r.Context(), id)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, vm)
	default:
		writeError(w, http.StatusNotFound, errNotFound)
	}
}

func withLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start).String())
	})
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
