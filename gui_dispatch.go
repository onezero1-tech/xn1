package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"
)

// runGUICmd is the `gui` command (and the bare double-click) entry point. On
// Windows it opens the native window; on every other platform it serves a
// browser dashboard instead, so a Linux build still has a graphical front-end.
func runGUICmd(ctx context.Context, opts options) error {
	if guiNativeAvailable {
		return runNativeGUI(ctx, opts)
	}
	return runWebDashboard(ctx, opts)
}

// runWebDashboard serves the embedded dashboard at the given port. It also
// polls the core status every 3s so the home page reflects a live session
// without the user reloading.
func runWebDashboard(ctx context.Context, opts options) error {
	port := opts.mixedPort
	if port == 0 {
		port = 7891
	}
	addr := fmt.Sprintf("127.0.0.1:%d", port)

	mux := http.NewServeMux()
	mux.HandleFunc("/", serveDashboard)
	mux.HandleFunc("/api/status", handleStatus)
	mux.HandleFunc("/api/about", handleAbout)
	mux.HandleFunc("/api/logs", handleLogs)
	mux.HandleFunc("/api/settings", handleSettings)
	mux.HandleFunc("/api/connect", handleConnect)
	mux.HandleFunc("/api/disconnect", handleDisconnect)

	srv := &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	fmt.Fprintln(os.Stderr, fmt.Sprintf("dashboard: http://%s  (Ctrl+C to stop)", addr))
	go func() {
		select {
		case <-ctx.Done():
			_ = srv.Close()
		case <-time.After(3 * time.Second):
		}
		// Best-effort browser open; silent on failure.
		openBrowser("http://" + addr)
	}()

	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("dashboard: %w", err)
	}
	return nil
}
