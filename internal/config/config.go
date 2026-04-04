package config

import (
	"encoding/json"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

type Config struct {
	Token            string `toml:"token"`
	APIURL           string `toml:"api_url"`
	MetricsInterval  int    `toml:"metrics_interval"`
	HostnameOverride string `toml:"hostname_override"`
}

func DefaultConfig() Config {
	return Config{
		APIURL:          "https://agent.plingpush.com",
		MetricsInterval: 60,
	}
}

func Load() (Config, error) {
	cfg := DefaultConfig()

	// Load config file (first found wins)
	paths := []string{
		"/etc/pling-agent/config.toml",
		filepath.Join(homeDir(), ".config", "pling-agent", "config.toml"),
	}

	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			if _, err := toml.DecodeFile(p, &cfg); err != nil {
				return cfg, err
			}
			break
		}
	}

	// Env vars override config file values
	if t := os.Getenv("PLING_TOKEN"); t != "" {
		cfg.Token = t
	}
	if u := os.Getenv("PLING_API_URL"); u != "" {
		cfg.APIURL = u
	}
	if h := os.Getenv("PLING_HOSTNAME"); h != "" {
		cfg.HostnameOverride = h
	}

	return cfg, nil
}

func (c Config) Hostname() string {
	if c.HostnameOverride != "" {
		return c.HostnameOverride
	}
	// On macOS, use scutil to get the Bonjour-resolvable hostname
	// so it matches what iOS discovers via mDNS.
	if out, err := exec.Command("scutil", "--get", "LocalHostName").Output(); err == nil {
		name := strings.TrimSpace(string(out))
		if name != "" {
			return name + ".local"
		}
	}
	name, _ := os.Hostname()
	return name
}

// HostAliases returns additional hostnames and IPs this machine is reachable at
// (e.g. Tailscale IPs, non-loopback interface IPs, os.Hostname).
func (c Config) HostAliases() []string {
	seen := map[string]bool{c.Hostname(): true}
	var aliases []string
	add := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" || seen[s] {
			return
		}
		seen[s] = true
		aliases = append(aliases, s)
	}

	// os.Hostname (might differ from Bonjour name)
	if name, err := os.Hostname(); err == nil {
		add(name)
	}

	// Tailscale status — get IPs and DNS name
	if out, err := exec.Command("tailscale", "status", "--json").Output(); err == nil {
		var ts struct {
			Self struct {
				TailscaleIPs []string `json:"TailscaleIPs"`
				DNSName      string   `json:"DNSName"`
			} `json:"Self"`
		}
		if json.Unmarshal(out, &ts) == nil {
			for _, ip := range ts.Self.TailscaleIPs {
				add(ip)
			}
			dns := strings.TrimSuffix(ts.Self.DNSName, ".")
			add(dns)
		}
	}

	// Non-loopback interface IPs
	if addrs, err := net.InterfaceAddrs(); err == nil {
		for _, a := range addrs {
			if ipnet, ok := a.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
				add(ipnet.IP.String())
			}
		}
	}

	return aliases
}

func homeDir() string {
	if h := os.Getenv("HOME"); h != "" {
		return h
	}
	return "/root"
}
