// cloud-agent reports every 4 seconds; v0.2 optionally handles fixed, locally validated operations.
package main

import (
	"bytes"
	"cloudease-lite/agent/collector"
	"cloudease-lite/agent/management"
	"cloudease-lite/internal/privatefs"
	"cloudease-lite/shared"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
)

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
func main() {
	server := flag.String("server", env("CLOUDEASE_SERVER", ""), "Control plane URL, e.g. http://127.0.0.1:5050")
	token := flag.String("token", env("CLOUDEASE_TOKEN", ""), "Agent registration token; prefer --token-file for services")
	tokenFile := flag.String("token-file", env("CLOUDEASE_TOKEN_FILE", ""), "Path to an owner-readable token file")
	name := flag.String("name", "", "Hostname override")
	ip := flag.String("advertise-ip", "", "IP displayed in the dashboard (optional)")
	defaultDisk := "/"
	if runtime.GOOS == "windows" {
		defaultDisk = env("SystemDrive", "C:") + "\\"
	}
	disk := flag.String("disk", defaultDisk, "Filesystem to monitor")
	once := flag.Bool("once", false, "Collect and report one sample, then exit")
	allowHTTP := flag.Bool("allow-insecure-http", false, "Allow public HTTP for an isolated development environment only")
	manage := flag.Bool("management", false, "Enable Linux management with local allowlists (off by default)")
	roots := flag.String("file-roots", env("CLOUDEASE_FILE_ROOTS", ""), "Comma-separated allowed file roots")
	services := flag.String("allowed-services", env("CLOUDEASE_ALLOWED_SERVICES", ""), "Comma-separated systemd allowlist")
	websiteConfig := flag.String("website-config", env("CLOUDEASE_WEBSITE_CONFIG", ""), "Root-owned JSON allowlist for website management (off by default)")
	flag.Parse()
	if *tokenFile != "" {
		data, err := privatefs.ReadSecret(*tokenFile)
		if err != nil {
			log.Fatal("Unable to read token file: ", err)
		}
		*token = strings.TrimSpace(string(data))
	}
	u, err := shared.ControlURL(*server, *allowHTTP)
	if err != nil {
		log.Fatal("Invalid control plane: ", err)
	}
	if !shared.ValidKey(*token) {
		log.Fatal("--token or --token-file is required")
	}
	if *ip != "" && net.ParseIP(*ip) == nil {
		log.Fatal("--advertise-ip must be an IP address")
	}
	endpoint := u.String() + "/api/agent/report"
	// Reject redirects so an Agent token cannot be forwarded to another endpoint.
	client := &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	manager, err := management.New(*manage, *roots, *services)
	if err != nil {
		log.Fatal(err)
	}
	if err = manager.ConfigureWebsites(*websiteConfig); err != nil {
		log.Fatal(err)
	}
	log.Printf("Client startup: OS=%s %s", runtime.GOOS, manager)
	var worker sync.WaitGroup
	if *manage && runtime.GOOS == "linux" && !*once {
		worker.Add(1)
		go func() { defer worker.Done(); managementLoop(ctx, client, u.String(), *token, manager) }()
	}
	defer func() { stop(); worker.Wait() }()
	c := &collector.Collector{DiskPath: *disk}
	var snapshot shared.ManagementInfo
	var details shared.SystemInfo
	var snapshotMu sync.RWMutex
	var extensions bool
	refreshSnapshot := func() {
		enabled := supportsExtensions(ctx, client, u.String(), *token)
		snapshotCtx, cancel := context.WithTimeout(ctx, 4*time.Second)
		next := manager.Snapshot(snapshotCtx)
		cancel()
		if supportsWebsites(ctx, client, u.String(), *token) {
			websiteCtx, websiteCancel := context.WithTimeout(ctx, 8*time.Second)
			next.Websites = manager.WebsiteSnapshot(websiteCtx)
			websiteCancel()
		}
		system := collector.Details()
		snapshotMu.Lock()
		snapshot, details, extensions = next, system, enabled
		snapshotMu.Unlock()
	}
	if *once {
		refreshSnapshot()
	} else {
		worker.Add(1)
		go func() {
			defer worker.Done()
			refreshSnapshot()
			timer := time.NewTicker(30 * time.Second)
			defer timer.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-timer.C:
					refreshSnapshot()
				}
			}
		}()
	}
	send := func() error {
		report, err := c.Collect()
		if err != nil {
			return err
		}
		if *name != "" {
			report.Name = *name
		}
		report.IP = *ip
		snapshotMu.RLock()
		system, capabilities, supported := details, snapshot, extensions
		snapshotMu.RUnlock()
		if supported {
			report.System = &system
			report.Management = &capabilities
		}
		report.SentAt = time.Now().UnixMilli()
		nonce := make([]byte, 16)
		if _, err := rand.Read(nonce); err != nil {
			return fmt.Errorf("create report nonce: %w", err)
		}
		report.Nonce = base64.RawURLEncoding.EncodeToString(nonce)
		data, err := json.Marshal(report)
		if err != nil {
			return err
		}
		// Large optional lists must never interrupt the four-second base metrics.
		if len(data) > 60*1024 && report.Management != nil {
			copyInfo := *report.Management
			copyInfo.Services = nil
			if copyInfo.Websites != nil {
				copyWeb := *copyInfo.Websites
				copyWeb.Checks = nil
				copyInfo.Websites = &copyWeb
			}
			report.Management = &copyInfo
			data, err = json.Marshal(report)
			if len(data) > 60*1024 {
				report.Management = nil
				data, err = json.Marshal(report)
			}
			if err != nil {
				return err
			}
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(data))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+*token)
		response, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("report request failed: %w", err)
		}
		defer response.Body.Close()
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 65536))
		if response.StatusCode != 200 {
			return fmt.Errorf("server returned HTTP %d", response.StatusCode)
		}
		log.Printf("Report accepted: CPU %.1f%% / RAM %.1f%% / Disk %.1f%%", report.CPU, report.Memory, report.Disk)
		return nil
	}
	ticker := time.NewTicker(4 * time.Second)
	defer ticker.Stop()
	if err := send(); err != nil {
		if *once {
			log.Fatal(err)
		}
		log.Print(err)
	}
	if *once {
		return
	}
	for {
		select {
		case <-ctx.Done():
			log.Print("Agent stopped")
			return
		case <-ticker.C:
			if err := send(); err != nil {
				log.Print(err)
			}
		}
	}
}
