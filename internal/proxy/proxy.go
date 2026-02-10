package proxy

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ThakerKush/Hatch/internal/store"
	"github.com/ThakerKush/Hatch/internal/vmm"
)

// Proxy is a subdomain-based reverse proxy that forwards requests to VMs and
// can wake (restore) snapshotted VMs on demand.
type Proxy struct {
	db          *store.DB
	vmm         *vmm.Manager
	baseDomain  string
	wakeTimeout time.Duration

	// lastAccess tracks the last proxy request time per subdomain.
	// Values are *atomic.Int64 (unix timestamp in seconds).
	lastAccess sync.Map

	// wakeMu serialises restore operations per VM so concurrent requests
	// don't all try to restore the same VM simultaneously.
	wakeMu sync.Map // map[string]*sync.Mutex
}

// New creates a Proxy that routes *.baseDomain to the appropriate VM.
func New(db *store.DB, mgr *vmm.Manager, baseDomain string, wakeTimeout time.Duration) *Proxy {
	return &Proxy{
		db:          db,
		vmm:         mgr,
		baseDomain:  strings.TrimPrefix(baseDomain, "."),
		wakeTimeout: wakeTimeout,
	}
}

// Handler returns an http.Handler suitable for http.Server.
func (p *Proxy) Handler() http.Handler {
	return http.HandlerFunc(p.serveHTTP)
}

func (p *Proxy) serveHTTP(w http.ResponseWriter, r *http.Request) {
	subdomain := p.extractSubdomain(r.Host)
	if subdomain == "" {
		http.Error(w, `{"error":"no subdomain in host header"}`, http.StatusBadGateway)
		return
	}

	// Look up the route.
	route, err := p.db.GetRouteBySubdomain(subdomain)
	if err != nil {
		slog.Error("proxy route lookup", "subdomain", subdomain, "error", err)
		http.Error(w, `{"error":"internal error"}`, http.StatusBadGateway)
		return
	}
	if route == nil {
		http.Error(w, fmt.Sprintf(`{"error":"no route for subdomain %q"}`, subdomain), http.StatusBadGateway)
		return
	}

	// Record access time for idle detection.
	p.recordAccess(subdomain)

	// Check VM state and possibly wake it.
	vm, ok := p.vmm.Get(route.VMID)
	if !ok {
		http.Error(w, `{"error":"vm not found"}`, http.StatusBadGateway)
		return
	}

	switch vm.State {
	case store.VMStateRunning:
		// happy path
	case store.VMStateSnapshotted:
		if !route.AutoWake {
			http.Error(w, `{"error":"vm is snapshotted and auto-wake is disabled"}`, http.StatusServiceUnavailable)
			return
		}
		if err := p.wakeVM(r.Context(), route.VMID); err != nil {
			slog.Error("wake vm failed", "vm", route.VMID, "error", err)
			http.Error(w, `{"error":"failed to wake vm"}`, http.StatusServiceUnavailable)
			return
		}
		// Re-fetch VM after restore to get the updated IP.
		vm, ok = p.vmm.Get(route.VMID)
		if !ok || vm.State != store.VMStateRunning {
			http.Error(w, `{"error":"vm not ready after wake"}`, http.StatusServiceUnavailable)
			return
		}
	default:
		http.Error(w, fmt.Sprintf(`{"error":"vm is in state %q, not proxying"}`, vm.State), http.StatusServiceUnavailable)
		return
	}

	if vm.GuestIP == "" {
		http.Error(w, `{"error":"vm has no guest IP"}`, http.StatusBadGateway)
		return
	}

	// Reverse-proxy the request to guest_ip:target_port.
	target := &url.URL{
		Scheme: "http",
		Host:   net.JoinHostPort(vm.GuestIP, fmt.Sprintf("%d", route.TargetPort)),
	}

	rp := &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			req.URL.Scheme = target.Scheme
			req.URL.Host = target.Host
			req.Host = r.Host // preserve original Host header
		},
	}

	rp.ServeHTTP(w, r)
}

// wakeVM restores a snapshotted VM, serialising concurrent wake requests
// for the same VM.
func (p *Proxy) wakeVM(ctx context.Context, vmID string) error {
	// Get or create a per-VM mutex.
	val, _ := p.wakeMu.LoadOrStore(vmID, &sync.Mutex{})
	mu := val.(*sync.Mutex)
	mu.Lock()
	defer mu.Unlock()

	// Re-check state under the lock: another request may have already restored it.
	vm, ok := p.vmm.Get(vmID)
	if !ok {
		return fmt.Errorf("vm not found: %s", vmID)
	}
	if vm.State == store.VMStateRunning {
		return nil // already restored by a concurrent request
	}

	ctx, cancel := context.WithTimeout(ctx, p.wakeTimeout)
	defer cancel()

	slog.Info("waking snapshotted vm", "vm", vmID)
	_, err := p.vmm.Restore(ctx, vmID)
	if err != nil {
		return fmt.Errorf("restore vm %s: %w", vmID, err)
	}

	// Allow a brief moment for the restored VM's network to come up.
	time.Sleep(500 * time.Millisecond)
	return nil
}

// extractSubdomain extracts the first label from a Host header like
// "my-agent.hatch.example.com" → "my-agent".
func (p *Proxy) extractSubdomain(host string) string {
	// Strip port if present.
	h, _, _ := net.SplitHostPort(host)
	if h == "" {
		h = host
	}

	suffix := "." + p.baseDomain
	if !strings.HasSuffix(h, suffix) {
		return ""
	}

	sub := strings.TrimSuffix(h, suffix)
	// sub might still contain dots (e.g. "a.b" in "a.b.hatch.local");
	// we only take the full prefix as the subdomain key.
	if sub == "" || strings.Contains(sub, ".") {
		return ""
	}
	return sub
}

// recordAccess updates the last-access timestamp for a subdomain.
func (p *Proxy) recordAccess(subdomain string) {
	now := time.Now().Unix()
	val, loaded := p.lastAccess.LoadOrStore(subdomain, &atomic.Int64{})
	ts := val.(*atomic.Int64)
	if !loaded {
		ts.Store(now)
	} else {
		ts.Store(now)
	}
}

// LastAccessTime returns the last request timestamp for a subdomain (unix seconds).
// Returns 0 if the subdomain has never been accessed through the proxy.
func (p *Proxy) LastAccessTime(subdomain string) int64 {
	val, ok := p.lastAccess.Load(subdomain)
	if !ok {
		return 0
	}
	return val.(*atomic.Int64).Load()
}
