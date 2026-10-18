package shared

import "testing"

func TestWebsiteValidation(t *testing.T) {
	good := WebsiteSpec{Name: "博客", Domain: "blog.example.com", RootPath: "/srv/blog", NginxConfig: "blog.conf"}
	if !ValidWebsite(good) {
		t.Fatal("valid site rejected")
	}
	for _, domain := range []string{"localhost", "169.254.169.254", "127.0.0.1", "site.local", "x.example.com/path", "X.example.com", "a;reboot.example", "-x.example.com", "x..example.com"} {
		if ValidDomain(domain) {
			t.Fatal(domain)
		}
	}
	for _, file := range []string{"../nginx.conf", "/etc/passwd", "--reload", "a\n.conf"} {
		bad := good
		bad.NginxConfig = file
		if ValidWebsite(bad) {
			t.Fatal(file)
		}
	}
	if !KnownAction("backup.database") || !MutatingAction("nginx.disable") || MutatingAction("ssl.check") {
		t.Fatal("action classification")
	}
}
