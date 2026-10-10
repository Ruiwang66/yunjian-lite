// Package config validates deployment settings before the server opens its database.
package config

import (
	"cloudease-lite/shared"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Listen                 string
	DataDir                string
	WebDir                 string
	CookieSecure           bool
	PublicURL              string
	TrustedProxies         []string
	RequireReportFreshness bool
	SessionTTL             time.Duration
	OfflineAfter           time.Duration
	RetentionDays          int
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
func Load() (Config, error) {
	days, err := strconv.Atoi(env("METRIC_RETENTION_DAYS", "7"))
	if err != nil || days < 1 || days > 365 {
		return Config{}, fmt.Errorf("METRIC_RETENTION_DAYS must be 1-365")
	}
	secure, err := strconv.ParseBool(env("COOKIE_SECURE", "false"))
	if err != nil {
		return Config{}, fmt.Errorf("COOKIE_SECURE must be a boolean")
	}
	legacy, err := strconv.ParseBool(env("ALLOW_LEGACY_REPORTS", "false"))
	if err != nil {
		return Config{}, fmt.Errorf("ALLOW_LEGACY_REPORTS must be a boolean")
	}
	cfg := Config{Listen: env("LISTEN_ADDR", "127.0.0.1:8080"), DataDir: env("DATA_DIR", "data"), WebDir: env("WEB_DIR", "web/dist"), CookieSecure: secure, SessionTTL: 24 * time.Hour, OfflineAfter: 90 * time.Second, RetentionDays: days, RequireReportFreshness: !legacy}
	if raw := os.Getenv("PUBLIC_URL"); raw != "" {
		u, e := shared.ControlURL(raw, false)
		if e != nil {
			return Config{}, fmt.Errorf("PUBLIC_URL: %w", e)
		}
		cfg.PublicURL = u.String()
		if u.Scheme == "https" {
			cfg.CookieSecure = true
		}
	}
	for _, raw := range strings.Split(os.Getenv("TRUSTED_PROXY_CIDRS"), ",") {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		if _, network, err := net.ParseCIDR(raw); err != nil {
			return Config{}, fmt.Errorf("TRUSTED_PROXY_CIDRS requires explicit CIDRs")
		} else if ones, _ := network.Mask.Size(); ones == 0 {
			return Config{}, fmt.Errorf("TRUSTED_PROXY_CIDRS must not trust every address")
		}
		cfg.TrustedProxies = append(cfg.TrustedProxies, raw)
	}
	return cfg, nil
}
