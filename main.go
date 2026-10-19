package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/awanish5101/distributed-database-proxy/proxy"
	"github.com/awanish5101/distributed-database-proxy/ratelimit"
	"github.com/awanish5101/distributed-database-proxy/router"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Println("[INFO] Initializing Distributed Database Proxy & Shard Router...")

	// 1. Initialize query router with 150 virtual nodes per physical shard
	r := router.NewRouter(150)

	// Register default shard topology
	r.RegisterShard(&router.ShardInfo{
		ID:       "shard-us-east-1",
		Primary:  "postgres-shard-1-primary:5432",
		Replicas: []string{"postgres-shard-1-replica:5432"},
		IsAlive:  true,
	})
	r.RegisterShard(&router.ShardInfo{
		ID:       "shard-us-west-2",
		Primary:  "postgres-shard-2-primary:5432",
		Replicas: []string{"postgres-shard-2-replica:5432"},
		IsAlive:  true,
	})

	// 2. Initialize token bucket rate limiter (100 req/sec burst, 50 req/sec sustained)
	limiter := ratelimit.NewTokenBucketLimiter(100, 50)

	// 3. Initialize proxy server
	proxyServer := proxy.NewProxyServer(r, limiter)
	proxyServer.RegisterBackendPool("postgres-shard-1-primary:5432", 50)
	proxyServer.RegisterBackendPool("postgres-shard-1-replica:5432", 50)
	proxyServer.RegisterBackendPool("postgres-shard-2-primary:5432", 50)
	proxyServer.RegisterBackendPool("postgres-shard-2-replica:5432", 50)

	mux := http.NewServeMux()
	mux.HandleFunc("/query", proxyServer.HandleQuery)
	mux.HandleFunc("/healthz", proxyServer.HandleHealth)
	mux.HandleFunc("/metrics", proxyServer.HandleMetrics)

	server := &http.Server{
		Addr:         ":" + port,
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	go func() {
		log.Printf("[INFO] Database Proxy listening on port %s", port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("[FATAL] Proxy server failed: %v", err)
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("[INFO] Shutting down Database Proxy gracefully...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Fatalf("[ERROR] Forced shutdown: %v", err)
	}
	log.Println("[INFO] Server exited successfully.")
}
