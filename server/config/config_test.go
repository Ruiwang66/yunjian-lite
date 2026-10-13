package config

import "testing"

func cleanEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{"PUBLIC_URL", "COOKIE_SECURE", "METRIC_RETENTION_DAYS", "LISTEN_ADDR", "TRUSTED_PROXY_CIDRS", "ALLOW_LEGACY_REPORTS"} {
		t.Setenv(k, "")
	}
}
func TestSecureDefaults(t *testing.T) {
	cleanEnv(t)
	cfg, err := Load()
	if err != nil || cfg.Listen != "127.0.0.1:8080" || !cfg.RequireReportFreshness || len(cfg.TrustedProxies) != 0 || cfg.RetentionDays != 30 {
		t.Fatalf("defaults: %+v %v", cfg, err)
	}
	t.Setenv("PUBLIC_URL", "https://monitor.example/")
	cfg, err = Load()
	if err != nil || !cfg.CookieSecure || cfg.PublicURL != "https://monitor.example" {
		t.Fatalf("HTTPS: %+v %v", cfg, err)
	}
	t.Setenv("TRUSTED_PROXY_CIDRS", "172.18.0.1/32, 127.0.0.1/32")
	cfg, err = Load()
	if err != nil || len(cfg.TrustedProxies) != 2 {
		t.Fatalf("proxies: %+v %v", cfg, err)
	}
	t.Setenv("ALLOW_LEGACY_REPORTS", "true")
	cfg, err = Load()
	if err != nil || cfg.RequireReportFreshness {
		t.Fatal("migration setting")
	}
}
func TestInvalidSettingsFailClosed(t *testing.T) {
	for _, item := range [][2]string{{"PUBLIC_URL", "http://monitor.example"}, {"METRIC_RETENTION_DAYS", "0"}, {"METRIC_RETENTION_DAYS", "999"}, {"TRUSTED_PROXY_CIDRS", "proxy.example"}, {"TRUSTED_PROXY_CIDRS", "0.0.0.0/0"}, {"TRUSTED_PROXY_CIDRS", "::/0"}, {"COOKIE_SECURE", "invalid"}, {"ALLOW_LEGACY_REPORTS", "invalid"}} {
		t.Run(item[0]+item[1], func(t *testing.T) {
			cleanEnv(t)
			t.Setenv(item[0], item[1])
			if _, err := Load(); err == nil {
				t.Fatal("invalid configuration accepted")
			}
		})
	}
}
