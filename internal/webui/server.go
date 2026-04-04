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

	// Filter aliases: only show useful ones (IPv4, Tailscale DNS, non-link-local)
	var filtered []string
	for _, a := range aliases {
		ip := net.ParseIP(a)
		if ip != nil {
			if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
				continue
			}
			// Skip ULA (fd00::/8, fc00::/7) except Tailscale
			if len(ip) == net.IPv6len && ip.To4() == nil {
				if ip[0] == 0xfd || ip[0] == 0xfc {
					continue
				}
			}
		}
		filtered = append(filtered, a)
	}
	aliasHTML := ""
	for _, a := range filtered {
		aliasHTML += `<span class="tag">` + a + `</span>`
	}

	remoteChecked := ""
	if cfg.AllowRemoteCommands {
		remoteChecked = "checked"
	}

	return `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>Pling Agent</title>
<style>
:root{--bg:#111;--surface:#191919;--border:#252525;--fg:#e0e0e0;--dim:#777;--accent:#a78bfa;--green:#34d399;--font:-apple-system,BlinkMacSystemFont,"SF Pro Text","Inter",system-ui,sans-serif;--mono:"SF Mono","Geist Mono",ui-monospace,monospace}
*{margin:0;padding:0;box-sizing:border-box}
body{font-family:var(--font);background:var(--bg);color:var(--fg);min-height:100vh;-webkit-font-smoothing:antialiased}
.wrap{max-width:480px;margin:0 auto;padding:32px 20px 48px}

/* Header */
header{margin-bottom:32px}
header h1{font-size:15px;font-weight:600;letter-spacing:-.01em;display:flex;align-items:center;gap:10px}
header h1 span{font-size:11px;font-weight:500;color:var(--dim);background:var(--surface);border:1px solid var(--border);padding:1px 7px;border-radius:4px;font-family:var(--mono)}
.host{font-size:13px;color:var(--dim);margin-top:4px;font-family:var(--mono)}
.indicator{width:6px;height:6px;border-radius:50%;background:var(--green);display:inline-block;margin-right:2px}

/* Section */
section{margin-bottom:24px}
section h2{font-size:11px;font-weight:600;color:var(--dim);text-transform:uppercase;letter-spacing:.08em;margin-bottom:8px;padding-left:2px}

/* Card */
.card{background:var(--surface);border:1px solid var(--border);border-radius:10px;overflow:hidden}
.field{display:flex;align-items:center;justify-content:space-between;padding:10px 14px;min-height:44px}
.field+.field{border-top:1px solid var(--border)}
.field .label{font-size:13px;color:var(--fg)}
.field .sub{font-size:11px;color:var(--dim);margin-top:1px}
.field .value{font-size:13px;color:var(--dim);font-family:var(--mono);text-align:right;max-width:55%;word-break:break-all}
.field .value.accent{color:var(--accent)}
.field input[type=text],.field input[type=number]{background:var(--bg);border:1px solid var(--border);border-radius:6px;padding:5px 8px;color:var(--fg);font-size:12px;font-family:var(--mono);width:160px;text-align:right}
.field input:focus{outline:none;border-color:var(--accent)}

/* Tags */
.tags{display:flex;flex-wrap:wrap;gap:5px;padding:8px 14px 12px}
.tag{font-size:11px;font-family:var(--mono);color:var(--dim);background:var(--bg);padding:2px 8px;border-radius:4px;border:1px solid var(--border)}

/* Toggle */
.sw{position:relative;width:38px;height:22px;flex-shrink:0}
.sw input{opacity:0;width:0;height:0}
.sw b{position:absolute;inset:0;background:#333;border-radius:11px;cursor:pointer;transition:.2s}
.sw b::after{content:"";position:absolute;width:16px;height:16px;left:3px;top:3px;background:#fff;border-radius:50%;transition:.2s}
.sw input:checked+b{background:var(--accent)}
.sw input:checked+b::after{transform:translateX(16px)}

/* Button */
.btn{display:block;width:100%;padding:10px;background:var(--accent);color:#000;border:none;border-radius:8px;font-size:13px;font-weight:600;font-family:var(--font);cursor:pointer;margin-top:8px;transition:opacity .15s}
.btn:hover{opacity:.85}
.btn:disabled{opacity:.3;cursor:default}

/* Toast */
.toast{position:fixed;bottom:20px;left:50%;transform:translateX(-50%) translateY(12px);background:var(--green);color:#000;padding:6px 18px;border-radius:6px;font-size:12px;font-weight:600;opacity:0;transition:.25s;pointer-events:none}
.toast.on{opacity:1;transform:translateX(-50%) translateY(0)}
</style>
</head>
<body>
<div class="wrap">

<header>
  <h1>Pling Agent <span>` + version + `</span></h1>
  <div class="host"><span class="indicator"></span> ` + hostname + `</div>
</header>

<section>
  <h2>Network</h2>
  <div class="card">
    <div class="field"><span class="label">Hostname</span><span class="value accent">` + hostname + `</span></div>
  </div>
  <div class="tags">` + aliasHTML + `</div>
</section>

<section>
  <h2>Permissions</h2>
  <div class="card">
    <div class="field">
      <div><span class="label">Scheduled commands</span><div class="sub">Execute commands sent from the Pling app</div></div>
      <label class="sw"><input type="checkbox" id="allowRemote" ` + remoteChecked + `><b></b></label>
    </div>
  </div>
</section>

<section>
  <h2>Settings</h2>
  <div class="card">
    <div class="field">
      <span class="label">Metrics interval</span>
      <input type="number" id="interval" value="` + strconv.Itoa(cfg.MetricsInterval) + `" min="10" max="3600">
    </div>
    <div class="field">
      <span class="label">Hostname override</span>
      <input type="text" id="hostnameOverride" value="` + cfg.HostnameOverride + `" placeholder="auto">
    </div>
  </div>
</section>

<section>
  <h2>Authentication</h2>
  <div class="card">
    <div class="field">
      <div><span class="label">API token</span><div class="sub">` + maskToken(cfg.Token) + `</div></div>
      <input type="text" id="apiToken" value="" placeholder="paste new token">
    </div>
    <div class="field"><span class="label">API endpoint</span><span class="value">` + cfg.APIURL + `</span></div>
  </div>
</section>

<button class="btn" id="saveBtn" onclick="save()">Save</button>
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
      const t=document.getElementById('toast');t.classList.add('on');setTimeout(()=>t.classList.remove('on'),1800);
      if(tok.length>0) setTimeout(()=>location.reload(),500);
    }
  }catch(e){console.error(e)}
  btn.disabled=false;
}
</script>
</body>
</html>`
}
