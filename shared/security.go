package shared

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
)

// ControlURL rejects credentials, paths and insecure public HTTP by default.
// An explicit development override never disables certificate verification.
func ControlURL(raw string, allowInsecure bool) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.RawFragment != "" || (u.Path != "" && u.Path != "/") || u.RawPath != "" || strings.ContainsAny(raw, "\r\n\t ") {
		return nil, fmt.Errorf("use an http(s) origin without credentials, path, query or fragment")
	}
	if port := u.Port(); port != "" {
		n, e := strconv.Atoi(port)
		if e != nil || n < 1 || n > 65535 {
			return nil, fmt.Errorf("invalid port")
		}
	}
	host := u.Hostname()
	ip := net.ParseIP(host)
	loopback := strings.EqualFold(host, "localhost") || (ip != nil && ip.IsLoopback())
	if u.Scheme == "http" && !loopback && !allowInsecure {
		return nil, fmt.Errorf("public control planes require HTTPS; HTTP is allowed only on loopback")
	}
	u.Path = ""
	u.Host = strings.ToLower(u.Host)
	return u, nil
}

func ValidKey(key string) bool {
	if len(key) != 43 {
		return false
	}
	for _, c := range key {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '-') {
			return false
		}
	}
	return true
}
