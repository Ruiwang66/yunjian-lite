package management

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync/atomic"
	"testing"
)

func TestV05ProbeRejectsInternalAndIPv6TransitionAddresses(t *testing.T) {
	for _, ip := range []string{"127.0.0.1", "169.254.169.254", "10.0.0.1", "::1", "fe80::1", "fc00::1", "fec0::1", "::127.0.0.1", "::ffff:127.0.0.1", "64:ff9b::a9fe:a9fe", "2001::1", "100::1"} {
		if publicAddress(net.ParseIP(ip)) {
			t.Error("non-public/transition target accepted", ip)
		}
	}
	for _, ip := range []string{"8.8.8.8", "2606:4700:4700::1111"} {
		if !publicAddress(net.ParseIP(ip)) {
			t.Error("public target rejected", ip)
		}
	}
}

func TestV05ProbePinsDNSAndRejectsRedirectsAndProxyEnvironment(t *testing.T) {
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:9")
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:9")
	var visited atomic.Int32
	local := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		visited.Add(1)
		if r.URL.Path != "/" {
			t.Error("followed untrusted redirect", r.URL.Path)
		}
		w.Header().Set("Location", "http://127.0.0.1/metadata-mock")
		w.WriteHeader(http.StatusFound)
	}))
	defer local.Close()
	for _, ip := range []string{"8.8.8.8", "2606:4700:4700::1111"} {
		var resolves int
		var addresses []string
		resolve := func(context.Context, string) ([]net.IPAddr, error) {
			resolves++
			if resolves > 1 {
				return []net.IPAddr{{IP: net.ParseIP("127.0.0.1")}}, nil
			}
			return []net.IPAddr{{IP: net.ParseIP(ip)}}, nil
		}
		connect := func(ctx context.Context, network, address string) (net.Conn, error) {
			addresses = append(addresses, address)
			// The test maps a simulated public peer exclusively to our loopback
			// mock. No connection to the public address or metadata service occurs.
			return (&net.Dialer{}).DialContext(ctx, network, local.Listener.Addr().String())
		}
		check := probeWebsiteUsing(context.Background(), "fixture.example", resolve, connect)
		want := []string{net.JoinHostPort(ip, "443"), net.JoinHostPort(ip, "80")}
		if resolves != 1 || !reflect.DeepEqual(addresses, want) || check.HTTPStatus != 302 || check.TLS.Status != "异常" {
			t.Fatalf("DNS/proxy/redirect/TLS boundary: resolves=%d addresses=%v result=%+v", resolves, addresses, check)
		}
	}
	if visited.Load() != 2 {
		t.Fatal("unexpected redirect visits", visited.Load())
	}
	var connected bool
	private := probeWebsiteUsing(context.Background(), "internal.example", func(context.Context, string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("10.0.0.1")}}, nil
	}, func(context.Context, string, string) (net.Conn, error) { connected = true; return nil, nil })
	if connected || private.Error == "" {
		t.Fatal("private website policy silently expanded")
	}
}
