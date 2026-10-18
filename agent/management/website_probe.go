package management

import (
	"cloudease-lite/shared"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Resolve once and pin the validated addresses for every connection. Redirects
// and proxy environment variables are intentionally excluded from site checks.
func publicAddress(ip net.IP) bool {
	if ip == nil || !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
		return false
	}
	for _, cidr := range []string{"0.0.0.0/8", "100.64.0.0/10", "192.0.0.0/24", "192.0.2.0/24", "198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24", "240.0.0.0/4", "2001:db8::/32", "64:ff9b::/96", "64:ff9b:1::/48", "2002::/16"} {
		_, block, _ := net.ParseCIDR(cidr)
		if block.Contains(ip) {
			return false
		}
	}
	return true
}
func probeWebsite(ctx context.Context, domain string) shared.WebsiteCheck {
	result := shared.WebsiteCheck{Domain: domain, Status: "异常", A: []string{}, AAAA: []string{}, Warnings: []string{}, CheckedAt: time.Now().UnixMilli(), TLS: shared.TLSInfo{Status: "未检测", Chain: []string{}}}
	if !shared.ValidDomain(domain) {
		result.Error = "域名无效"
		return result
	}
	ctx, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, domain)
	if err != nil || len(ips) == 0 {
		result.Status = "解析失败"
		result.Error = "DNS 解析失败"
		return result
	}
	if len(ips) > 32 {
		result.Error = "DNS 地址数量超过限制"
		return result
	}
	for _, ip := range ips {
		if !publicAddress(ip.IP) {
			result.Error = "域名解析到非公网地址，已拒绝访问"
			return result
		}
		if ip.IP.To4() != nil {
			result.A = append(result.A, ip.IP.String())
		} else {
			result.AAAA = append(result.AAAA, ip.IP.String())
		}
	}
	dial := func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil || host != domain || (port != "443" && port != "80") {
			return nil, errors.New("unexpected probe target")
		}
		var last error
		for _, ip := range ips {
			conn, e := (&net.Dialer{Timeout: 2 * time.Second}).DialContext(ctx, "tcp", net.JoinHostPort(ip.IP.String(), port))
			if e == nil {
				return conn, nil
			}
			last = e
		}
		return nil, last
	}
	transport := &http.Transport{DialContext: dial, TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12, ServerName: domain}, TLSHandshakeTimeout: 3 * time.Second, ResponseHeaderTimeout: 3 * time.Second, MaxResponseHeaderBytes: 32 * 1024, DisableKeepAlives: true}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 6 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	started := time.Now()
	req, _ := http.NewRequestWithContext(ctx, "GET", "https://"+domain+"/", nil)
	req.Header.Set("User-Agent", "CloudEase-Lite/"+shared.Version)
	response, err := client.Do(req)
	result.ResponseMS = time.Since(started).Milliseconds()
	if err != nil {
		result.Error = "HTTPS 访问失败（连接、超时或证书错误）"
		result.TLS.Status = "异常"
		result.TLS.Error = "未通过可信证书链、域名及有效期验证"
		// A failed verified handshake never causes insecure retries.
		var unknown x509.UnknownAuthorityError
		var invalid x509.CertificateInvalidError
		var mismatch x509.HostnameError
		if errors.As(err, &unknown) || errors.As(err, &invalid) || errors.As(err, &mismatch) {
			result.Error = "SSL 证书验证失败"
		}
		httpReq, _ := http.NewRequestWithContext(ctx, "HEAD", "http://"+domain+"/", nil)
		if fallback, e := client.Do(httpReq); e == nil {
			result.HTTPStatus = fallback.StatusCode
			fallback.Body.Close()
			result.Warnings = append(result.Warnings, "HTTP 可达，HTTPS 不可用或证书无效")
		}
		return result
	}
	defer response.Body.Close()
	io.Copy(io.Discard, io.LimitReader(response.Body, 1024))
	result.HTTPStatus = response.StatusCode
	result.Accessible = response.StatusCode >= 200 && response.StatusCode < 400
	if result.Accessible {
		result.Status = "正常"
	} else {
		result.Error = "HTTP " + strconv.Itoa(response.StatusCode)
	}
	if state := response.TLS; state != nil && len(state.PeerCertificates) > 0 {
		cert := state.PeerCertificates[0]
		result.TLS = shared.TLSInfo{Status: "有效", Issuer: shared.SafeLabel(cert.Issuer.CommonName), NotBefore: cert.NotBefore.UnixMilli(), NotAfter: cert.NotAfter.UnixMilli(), DaysLeft: int(time.Until(cert.NotAfter).Hours() / 24), Version: tls.VersionName(state.Version), Chain: []string{}}
		if result.TLS.Issuer == "" {
			result.TLS.Issuer = shared.SafeLabel(strings.Join(cert.Issuer.Organization, ", "))
		}
		for _, certificate := range state.PeerCertificates {
			if len(result.TLS.Chain) < 8 {
				result.TLS.Chain = append(result.TLS.Chain, shared.SafeLabel(certificate.Subject.String()))
			}
		}
		if result.TLS.DaysLeft <= 7 {
			result.TLS.Warning = "高危：证书将在 7 天内过期"
		} else if result.TLS.DaysLeft <= 30 {
			result.TLS.Warning = "提醒：证书将在 30 天内过期"
		}
	}
	return result
}
