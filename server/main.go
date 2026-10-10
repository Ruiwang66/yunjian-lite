// CloudEase Lite is a lightweight server monitoring control plane.
package main

import (
	"cloudease-lite/server/api"
	"cloudease-lite/server/config"
	"cloudease-lite/server/database"
	"cloudease-lite/server/service"
	live "cloudease-lite/server/websocket"
	"context"
	"flag"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
)

func main() {
	health := flag.Bool("healthcheck", false, "Probe the running server health and exit")
	reset := flag.Bool("reset-admin-password", false, "Generate a new admin password and revoke all sessions, then exit")
	flag.Parse()
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}
	if *health {
		host, port, err := net.SplitHostPort(cfg.Listen)
		if err != nil {
			log.Fatal(err)
		}
		if host == "" || host == "0.0.0.0" || host == "::" {
			host = "127.0.0.1"
		}
		client := &http.Client{Timeout: 5 * time.Second}
		response, err := client.Get("http://" + net.JoinHostPort(host, port) + "/healthz")
		if err != nil {
			log.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != 200 {
			log.Fatal("healthcheck failed: ", response.StatusCode)
		}
		return
	}
	db, err := database.Open(cfg.DataDir)
	if err != nil {
		log.Fatal(err)
	}
	sqlDB, _ := db.DB()
	defer sqlDB.Close()
	created, err := service.BootstrapAdmin(db, cfg.DataDir, *reset)
	if err != nil {
		log.Fatal(err)
	}
	if created {
		log.Printf("Admin password generated. Read %s", filepath.Join(cfg.DataDir, "admin-password.txt"))
	}
	if *reset {
		return
	}
	hub := live.New()
	defer hub.Close()
	router := api.New(db, cfg, hub).Router()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	done := make(chan struct{})
	go func() {
		defer close(done)
		service.Monitor(ctx, db, cfg.OfflineAfter, cfg.RetentionDays, hub.Broadcast)
	}()
	server := &http.Server{Addr: cfg.Listen, Handler: router, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 20 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 * 1024}
	go func() {
		log.Printf("CloudEase Lite v0.1 listening on %s", cfg.Listen)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("HTTP server: %v", err)
			stop()
		}
	}()
	<-ctx.Done()
	hub.Close()
	shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdown); err != nil {
		log.Printf("shutdown: %v", err)
	}
	<-done
}
