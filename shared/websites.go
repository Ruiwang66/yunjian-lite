package shared

import (
	"net"
	"path"
	"regexp"
	"strings"
)

type WebsiteSpec struct {
	Name        string `json:"name"`
	Domain      string `json:"domain"`
	RootPath    string `json:"root_path"`
	NginxConfig string `json:"nginx_config"`
}
type NginxInfo struct {
	Available bool   `json:"available"`
	Version   string `json:"version"`
	State     string `json:"state"`
}
type PHPInfo struct {
	Available  bool     `json:"available"`
	Version    string   `json:"version"`
	FPMState   string   `json:"fpm_state"`
	Extensions []string `json:"extensions"`
	Warnings   []string `json:"warnings"`
}
type WebsiteInventory struct {
	Demo    bool           `json:"demo,omitempty"`
	Enabled bool           `json:"enabled"`
	Nginx   NginxInfo      `json:"nginx"`
	PHP     PHPInfo        `json:"php"`
	Sites   []WebsiteSpec  `json:"sites"`
	Checks  []WebsiteCheck `json:"checks"`
}
type TLSInfo struct {
	Status    string   `json:"status"`
	Issuer    string   `json:"issuer"`
	NotBefore int64    `json:"not_before"`
	NotAfter  int64    `json:"not_after"`
	DaysLeft  int      `json:"days_left"`
	Version   string   `json:"version"`
	Chain     []string `json:"chain"`
	Warning   string   `json:"warning"`
	Error     string   `json:"error,omitempty"`
}
type WebsiteCheck struct {
	Domain     string   `json:"domain"`
	Status     string   `json:"status"`
	A          []string `json:"a"`
	AAAA       []string `json:"aaaa"`
	HTTPStatus int      `json:"http_status"`
	ResponseMS int64    `json:"response_ms"`
	Accessible bool     `json:"accessible"`
	TLS        TLSInfo  `json:"tls"`
	Warnings   []string `json:"warnings"`
	CheckedAt  int64    `json:"checked_at"`
	Error      string   `json:"error,omitempty"`
}
type SiteResult struct {
	Domain   string   `json:"domain"`
	Config   string   `json:"config,omitempty"`
	Enabled  bool     `json:"enabled"`
	Backup   string   `json:"backup,omitempty"`
	Warnings []string `json:"warnings"`
}
type BackupResult struct {
	Name      string `json:"name"`
	Size      int64  `json:"size"`
	Kind      string `json:"kind"`
	SHA256    string `json:"sha256"`
	CreatedAt int64  `json:"created_at"`
}

var siteFile = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,119}$`)

func ValidDomain(domain string) bool {
	if len(domain) > 253 || strings.ToLower(domain) != domain || !strings.Contains(domain, ".") || net.ParseIP(domain) != nil {
		return false
	}
	for _, part := range strings.Split(domain, ".") {
		if len(part) < 1 || len(part) > 63 || part[0] == '-' || part[len(part)-1] == '-' {
			return false
		}
		for _, c := range part {
			if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-') {
				return false
			}
		}
	}
	for _, suffix := range []string{".local", ".localhost", ".internal", ".lan", ".home", ".test", ".invalid"} {
		if strings.HasSuffix(domain, suffix) {
			return false
		}
	}
	return true
}
func ValidWebsite(s WebsiteSpec) bool {
	return ValidDomain(s.Domain) && len(s.Name) > 0 && len(s.Name) <= 120 && len(s.RootPath) <= 1024 && path.IsAbs(s.RootPath) && path.Clean(s.RootPath) == s.RootPath && s.RootPath != "/" && !strings.ContainsAny(s.RootPath, "\\\x00\r\n") && siteFile.MatchString(s.NginxConfig) && !strings.Contains(s.NginxConfig, "..")
}
func WebsiteAction(action string) bool {
	switch action {
	case "nginx.status", "nginx.sites", "nginx.read", "nginx.enable", "nginx.disable", "php.inspect", "website.check", "ssl.check", "domain.check", "backup.files", "backup.database":
		return true
	}
	return false
}
