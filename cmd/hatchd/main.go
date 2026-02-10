package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ThakerKush/Hatch/internal/api"
	"github.com/ThakerKush/Hatch/internal/config"
	"github.com/ThakerKush/Hatch/internal/proxy"
	"github.com/ThakerKush/Hatch/internal/store"
	"github.com/ThakerKush/Hatch/internal/vmm"
)

func main() {
	// Structured logger to stdout.
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))

	cfg := config.LoadFromEnv()
	slog.Info("configuration loaded",
		"http_addr", cfg.HTTPAddr,
		"proxy_addr", cfg.ProxyAddr,
		"data_dir", cfg.DataDir,
		"s3_enabled", cfg.S3Enabled(),
	)

	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		slog.Error("create data dir", "error", err)
		os.Exit(1)
	}

	// ── Database (PostgreSQL) ────────────────────────────────────────────
	db, err := store.OpenDB(cfg.DatabaseURL)
	if err != nil {
		slog.Error("open database", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	// ── S3 (optional) ────────────────────────────────────────────────────
	var s3Client *store.S3Client
	if cfg.S3Enabled() {
		s3Client, err = store.NewS3Client(store.S3Config{
			Endpoint:  cfg.S3Endpoint,
			Bucket:    cfg.S3Bucket,
			Region:    cfg.S3Region,
			AccessKey: cfg.S3AccessKey,
			SecretKey: cfg.S3SecretKey,
		})
		if err != nil {
			slog.Error("init S3 client", "error", err)
			os.Exit(1)
		}
		slog.Info("S3 snapshot storage enabled", "bucket", cfg.S3Bucket)
	} else {
		slog.Warn("S3 not configured; snapshot/restore will be unavailable")
	}

	// ── VM Manager ───────────────────────────────────────────────────────
	manager, err := vmm.NewManager(cfg, db, s3Client)
	if err != nil {
		slog.Error("init vm manager", "error", err)
		os.Exit(1)
	}
	defer manager.Shutdown()

	// ── API Server ───────────────────────────────────────────────────────
	apiServer := api.NewServer(cfg, db, manager)
	httpServer := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           apiServer.Routes(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		slog.Info("API server listening", "addr", cfg.HTTPAddr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("api listen", "error", err)
			os.Exit(1)
		}
	}()

	// ── Reverse Proxy ────────────────────────────────────────────────────
	proxyHandler := proxy.New(db, manager, cfg.ProxyBaseDomain, cfg.ProxyWakeTimeout)

	proxyServer := &http.Server{
		Addr:              cfg.ProxyAddr,
		Handler:           proxyHandler.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		slog.Info("reverse proxy listening", "addr", cfg.ProxyAddr, "base_domain", cfg.ProxyBaseDomain)
		if err := proxyServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("proxy listen", "error", err)
			os.Exit(1)
		}
	}()

	// ── Idle Monitor ─────────────────────────────────────────────────────
	var idleMon *proxy.IdleMonitor
	if cfg.S3Enabled() {
		idleMon = proxy.NewIdleMonitor(db, manager, proxyHandler, cfg.IdleCheckInterval, cfg.IdleTimeout)
		idleMon.Start()
	}

	// ── Graceful Shutdown ────────────────────────────────────────────────
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	slog.Info("shutting down...")

	if idleMon != nil {
		idleMon.Stop()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Drain both servers in parallel.
	done := make(chan struct{}, 2)
	go func() {
		if err := httpServer.Shutdown(ctx); err != nil {
			slog.Error("api shutdown", "error", err)
		}
		done <- struct{}{}
	}()
	go func() {
		if err := proxyServer.Shutdown(ctx); err != nil {
			slog.Error("proxy shutdown", "error", err)
		}
		done <- struct{}{}
	}()

	<-done
	<-done

	slog.Info("shutdown complete")
}
