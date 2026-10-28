//go:build !windows

package main

import (
	"context"
	"encoding/json"
	"embed"
	"fmt"
	"net/http"
	"os/exec"
	"strings"

	"github.com/xhwarp/warp-client/internal/core"
)

//go:embed web
var webFS embed.FS

// dashboardIndex is a minimal fallback page served when the embedded assets
// cannot be resolved (e.g. a `go run` without the web/ directory present).
const dashboardIndex = `<!DOCTYPE html><html><head><meta charset="utf-8"><title>XEHN</title></head>
<body style="font-family:system-ui;margin:40px;background:#1a1b26;color:#c0caf5">
<h2 style="color:#7aa2f7">XEHN dashboard</h2>
<p>Embedded web assets are missing from this build.</p>
<p>Rebuild with the <code>web/</code> directory present:</p>
<pre>go build -o xehn .</pre>
<p style="color:#565f77">Use the CLI in the meantime: <code>xehn connect</code></p>
</body></html>`

// serveDashboard serves the embedded static files. Falls back to a simple
// index when the embed is not compiled in (e.g. a `go run` without assets).
func serveDashboard(w http.ResponseWriter, r *http.Request) {
	// embed.FS stores paths as "web/index.html", "web/style.css" etc.
	// Serve the root at the browser root, and static assets at /<name>.
	if r.URL.Path == "/" {
		data, err := webFS.ReadFile("web/index.html")
		if err != nil {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(dashboardIndex))
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(data)
		return
	}
	// /style.css → web/style.css, /app.js → web/app.js
	name := strings.TrimPrefix(r.URL.Path, "/")
	full := "web/" + name
	data, err := webFS.ReadFile(full)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	// Best-effort content type from the extension.
	ct := "application/octet-stream"
	switch {
	case strings.HasSuffix(name, ".css"):
		ct = "text/css; charset=utf-8"
	case strings.HasSuffix(name, ".js"):
		ct = "application/javascript; charset=utf-8"
	case strings.HasSuffix(name, ".html"):
		ct = "text/html; charset=utf-8"
	case strings.HasSuffix(name, ".png"):
		ct = "image/png"
	case strings.HasSuffix(name, ".svg"):
		ct = "image/svg+xml"
	}
	w.Header().Set("Content-Type", ct)
	_, _ = w.Write(data)
}

// openBrowser tries to launch the system browser. Failures are silent: a
// headless server or a locked-down environment should not break the dashboard.
func openBrowser(url string) {
	for _, c := range []string{"xdg-open", "open", "start"} {
		if err := exec.Command(c, url).Start(); err == nil {
			return
		}
	}
}

// --- /api/status -------------------------------------------------------------

type statusResponse struct {
	Connected  bool   `json:"connected"`
	ExitIP     string `json:"exit_ip,omitempty"`
	Country    string `json:"country,omitempty"`
	Colo       string `json:"colo,omitempty"`
	Latency    string `json:"latency,omitempty"`
	Proto      string `json:"proto,omitempty"`
	Endpoint   string `json:"endpoint,omitempty"`
	MixedPort  int    `json:"mixed_port,omitempty"`
	TUN        bool   `json:"tun,omitempty"`
	Version    string `json:"version,omitempty"`
}

func handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "GET only", http.StatusMethodNotAllowed)
		return
	}
	st, err := loadState()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	resp := statusResponse{
		Connected:  st.Connected,
		ExitIP:     st.ExitIP,
		Country:    st.Country,
		Colo:       st.Colo,
		Proto:      st.Proto,
		Endpoint:   st.Endpoint,
		MixedPort:  st.MixedPort,
		TUN:        st.TUN,
		Version:    resolveVersion(),
	}
	if st.LatencyMS > 0 {
		resp.Latency = fmt.Sprintf("%dms", st.LatencyMS)
	}
	writeJSON(w, http.StatusOK, resp)
}

// --- /api/about --------------------------------------------------------------

func handleAbout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "GET only", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"product":     "XEHN",
		"version":     resolveVersion(),
		"repository":  repoURL,
		"releases":    repoReleasesURL,
		"core":        coreRepoURL,
		"mixed port":  fmt.Sprintf("%d", core.DefaultMixedPort),
		"api port":    fmt.Sprintf("%d", core.DefaultAPIPort),
	})
}

// --- /api/logs ---------------------------------------------------------------

type logEntry struct {
	Level string `json:"level"`
	Msg   string `json:"msg"`
}

func handleLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "GET only", http.StatusMethodNotAllowed)
		return
	}
	entries := []logEntry{}
	if st, err := loadState(); err == nil && st.CorePID > 0 {
		entries = append(entries, logEntry{Level: "info", Msg: fmt.Sprintf("core pid %d", st.CorePID)})
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"lines": entries})
}

// --- /api/settings -----------------------------------------------------------

func handleSettings(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"mixedPort":  core.DefaultMixedPort,
			"apiPort":    core.DefaultAPIPort,
			"proxyMode":  "rule",
		})
		return
	}
	if r.Method == http.MethodPost {
		writeJSON(w, http.StatusOK, map[string]string{"ok": "saved"})
		return
	}
	http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
}

// --- /api/connect & /api/disconnect ------------------------------------------

func handleConnect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Proto string `json:"proto"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	if body.Proto == "" {
		body.Proto = "awg"
	}
	opts := defaultConnectOptions(body.Proto, false, true)
	opts.mixedPort = core.DefaultMixedPort
	opts.apiPort = core.DefaultAPIPort
	err := runConnectCmd(context.Background(), opts)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"ok": "connecting"})
}

func handleDisconnect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	err := runDisconnectCmd(context.Background(), options{mixedPort: core.DefaultMixedPort})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"ok": "disconnected"})
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
