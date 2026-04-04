package webui

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jeramo/pling-agent/internal/config"
)

// Start launches the web UI on localhost.
func Start(ctx context.Context, cfg *config.Config, version string) {
	mux := http.NewServeMux()

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, page(cfg, version))
	})

	mux.HandleFunc("/api/config", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{
				"token":                 maskToken(cfg.Token),
				"api_url":              cfg.APIURL,
				"metrics_interval":     cfg.MetricsInterval,
				"hostname_override":    cfg.HostnameOverride,
				"hostname":            cfg.Hostname(),
				"aliases":             cfg.HostAliases(),
				"allow_remote_commands": cfg.AllowRemoteCommands,
				"webui_port":          cfg.WebUIPort,
				"config_path":         config.ConfigPath(),
				"version":             version,
			})
			return
		}

		if r.Method == "POST" {
			var body struct {
				MetricsInterval     *int    `json:"metrics_interval"`
				HostnameOverride    *string `json:"hostname_override"`
				AllowRemoteCommands *bool   `json:"allow_remote_commands"`
				Token               *string `json:"token"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, `{"error":"invalid json"}`, 400)
				return
			}

			if body.MetricsInterval != nil && *body.MetricsInterval >= 10 {
				cfg.MetricsInterval = *body.MetricsInterval
			}
			if body.HostnameOverride != nil {
				cfg.HostnameOverride = strings.TrimSpace(*body.HostnameOverride)
			}
			if body.AllowRemoteCommands != nil {
				cfg.AllowRemoteCommands = *body.AllowRemoteCommands
			}
			if body.Token != nil && len(*body.Token) > 0 {
				cfg.Token = strings.TrimSpace(*body.Token)
			}

			if err := cfg.Save(); err != nil {
				http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), 500)
				return
			}

			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"ok":true}`)
			return
		}

		http.Error(w, "method not allowed", 405)
	})

	// Auth middleware — public internet requires token, local/private/Tailscale is trusted
	authPin := ""
	if len(cfg.Token) >= 16 {
		authPin = cfg.Token[:16]
	}
	authedMux := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := r.RemoteAddr
		if host, _, err := net.SplitHostPort(ip); err == nil {
			ip = host
		}
		if !isTrustedIP(ip) && r.URL.Query().Get("token") != authPin {
			http.Error(w, "unauthorized — append ?token=<first 16 chars of your API token>", 401)
			return
		}
		mux.ServeHTTP(w, r)
	})

	addr := fmt.Sprintf("0.0.0.0:%d", cfg.WebUIPort)
	srv := &http.Server{Addr: addr, Handler: authedMux, ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second}

	go func() {
		<-ctx.Done()
		srv.Close()
	}()

	log.Printf("[webui] listening on http://%s (local access: no auth, remote: ?token=... required)", addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Printf("[webui] failed to start: %v", err)
	}
}

// isTrustedIP returns true for localhost, private LAN, and Tailscale ranges.
func isTrustedIP(ip string) bool {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return false
	}
	// Localhost
	if parsed.IsLoopback() {
		return true
	}
	// Private ranges: 10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16
	if parsed.IsPrivate() {
		return true
	}
	// Tailscale CGNAT: 100.64.0.0/10
	if ip4 := parsed.To4(); ip4 != nil && ip4[0] == 100 && ip4[1] >= 64 && ip4[1] <= 127 {
		return true
	}
	// Link-local
	if parsed.IsLinkLocalUnicast() {
		return true
	}
	return false
}

func maskToken(t string) string {
	if len(t) <= 8 {
		return "••••"
	}
	return t[:4] + "••••" + t[len(t)-4:]
}

func page(cfg *config.Config, version string) string {
	hostname := cfg.Hostname()
	aliases := cfg.HostAliases()
	aliasHTML := ""
	for _, a := range aliases {
		aliasHTML += `<span class="alias">` + a + `</span>`
	}

	remoteChecked := ""
	if cfg.AllowRemoteCommands {
		remoteChecked = "checked"
	}

	return `<!DOCTYPE html>
<html>
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Pling Agent</title>
<style>
*{margin:0;padding:0;box-sizing:border-box}
body{font-family:-apple-system,BlinkMacSystemFont,"SF Pro Text",system-ui,sans-serif;background:#0d0f14;color:#e4e4e7;min-height:100vh;padding:24px}
.container{max-width:520px;margin:0 auto}
h1{font-size:20px;font-weight:700;margin-bottom:4px;display:flex;align-items:center;gap:8px}
.version{font-size:12px;font-weight:500;color:#71717a;background:#27272a;padding:2px 8px;border-radius:6px}
.subtitle{font-size:13px;color:#71717a;margin-bottom:24px}
.card{background:#18181b;border:1px solid #27272a;border-radius:12px;padding:16px;margin-bottom:12px}
.card h2{font-size:13px;font-weight:600;color:#a1a1aa;text-transform:uppercase;letter-spacing:1px;margin-bottom:12px}
.row{display:flex;justify-content:space-between;align-items:center;padding:8px 0}
.row:not(:last-child){border-bottom:1px solid #27272a}
.row label{font-size:14px;color:#e4e4e7}
.row .hint{font-size:11px;color:#71717a;margin-top:2px}
.row input[type=text],.row input[type=number]{background:#0d0f14;border:1px solid #3f3f46;border-radius:6px;padding:6px 10px;color:#e4e4e7;font-size:13px;width:180px;text-align:right;font-family:ui-monospace,monospace}
.row input:focus{outline:none;border-color:#a78bfa}
.toggle{position:relative;width:42px;height:24px;cursor:pointer}
.toggle input{opacity:0;width:0;height:0}
.toggle .slider{position:absolute;inset:0;background:#3f3f46;border-radius:12px;transition:.2s}
.toggle input:checked+.slider{background:#a78bfa}
.toggle .slider::before{content:"";position:absolute;height:18px;width:18px;left:3px;bottom:3px;background:#fff;border-radius:50%;transition:.2s}
.toggle input:checked+.slider::before{transform:translateX(18px)}
.aliases{display:flex;flex-wrap:wrap;gap:6px;margin-top:8px}
.alias{font-size:11px;font-family:ui-monospace,monospace;background:#27272a;color:#a1a1aa;padding:3px 8px;border-radius:6px}
.hostname{font-size:14px;font-family:ui-monospace,monospace;color:#a78bfa}
.path{font-size:11px;font-family:ui-monospace,monospace;color:#71717a;word-break:break-all}
.status{display:flex;align-items:center;gap:6px;font-size:13px;color:#4ade80}
.status .dot{width:8px;height:8px;border-radius:50%;background:#4ade80}
.btn{background:#a78bfa;color:#000;border:none;padding:8px 20px;border-radius:8px;font-size:13px;font-weight:600;cursor:pointer;margin-top:16px;width:100%}
.btn:hover{background:#8b5cf6}
.btn:disabled{opacity:.4;cursor:default}
.toast{position:fixed;bottom:24px;left:50%;transform:translateX(-50%);background:#4ade80;color:#000;padding:8px 20px;border-radius:8px;font-size:13px;font-weight:600;opacity:0;transition:.3s}
.toast.show{opacity:1}
</style>
</head>
<body>
<div class="container">
<h1>⚡ Pling Agent <span class="version">v` + version + `</span></h1>
<p class="subtitle">Running on ` + hostname + `</p>

<div class="card">
  <h2>Status</h2>
  <div class="row"><label>State</label><span class="status"><span class="dot"></span>Running</span></div>
  <div class="row"><label>Hostname</label><span class="hostname">` + hostname + `</span></div>
  <div class="row">
    <label>Aliases</label>
  </div>
  <div class="aliases">` + aliasHTML + `</div>
</div>

<div class="card">
  <h2>Permissions</h2>
  <div class="row">
    <div><label>Scheduled commands</label><div class="hint">Allow the agent to execute commands from Pling</div></div>
    <label class="toggle"><input type="checkbox" id="allowRemote" ` + remoteChecked + `><span class="slider"></span></label>
  </div>
</div>

<div class="card">
  <h2>Settings</h2>
  <div class="row">
    <label>Metrics interval (sec)</label>
    <input type="number" id="interval" value="` + strconv.Itoa(cfg.MetricsInterval) + `" min="10" max="3600">
  </div>
  <div class="row">
    <label>Hostname override</label>
    <input type="text" id="hostnameOverride" value="` + cfg.HostnameOverride + `" placeholder="auto-detect">
  </div>
</div>

<div class="card">
  <h2>Config</h2>
  <div class="row">
    <div><label>API token</label><div class="hint">` + maskToken(cfg.Token) + `</div></div>
    <input type="text" id="apiToken" value="" placeholder="paste new token">
  </div>
  <div class="row"><label>API URL</label><span class="path">` + cfg.APIURL + `</span></div>
  <div class="row"><label>Config file</label><span class="path">` + config.ConfigPath() + `</span></div>
</div>

<button class="btn" id="saveBtn" onclick="save()">Save changes</button>
<div class="toast" id="toast">Saved</div>
</div>

<script>
async function save(){
  const btn=document.getElementById('saveBtn');
  btn.disabled=true;
  const body={
    metrics_interval:parseInt(document.getElementById('interval').value)||60,
    hostname_override:document.getElementById('hostnameOverride').value,
    allow_remote_commands:document.getElementById('allowRemote').checked,
  };
  const tok=document.getElementById('apiToken').value.trim();
  if(tok.length>0) body.token=tok;
  try{
    const r=await fetch('/api/config',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(body)});
    if(r.ok){
      document.getElementById('apiToken').value='';
      const t=document.getElementById('toast');t.classList.add('show');setTimeout(()=>t.classList.remove('show'),2000);
      if(tok.length>0) setTimeout(()=>location.reload(),500);
    }
  }catch(e){console.error(e)}
  btn.disabled=false;
}
</script>
</body>
</html>`
}
