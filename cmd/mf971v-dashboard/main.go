// Command mf971v-dashboard serves a read-only status dashboard for a ZTE
// MF971V. It never writes to the device and binds to loopback unless told
// otherwise.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"mf971v-dashboard/internal/telemetry"
	"mf971v-dashboard/internal/webui"
)

var version = "dev"

func main() {
	var (
		listen   = flag.String("listen", "127.0.0.1:8080", "HTTP listen address; loopback is strongly recommended")
		router   = flag.String("router", "http://192.168.0.1", "vendor web origin to poll")
		interval = flag.Duration("interval", 2*time.Second, "polling interval for the status snapshot")
		showVer  = flag.Bool("version", false, "print version and exit")
	)
	flag.Parse()

	if *showVer {
		fmt.Println(version)
		return
	}
	if *interval < 500*time.Millisecond {
		log.Fatalf("-interval must be at least 500ms")
	}
	if host, _, err := net.SplitHostPort(*listen); err == nil {
		if ip := net.ParseIP(host); ip != nil && !ip.IsLoopback() {
			log.Printf("WARNING: listening on %s exposes read-only device telemetry to the network; set an access policy", host)
		}
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	sampler := telemetry.NewSampler(telemetry.NewAPI(*router))

	// Prime once so the first page load has data instead of a spinner.
	first := sampler.Snapshot(ctx)
	if len(first.Warnings) > 0 {
		log.Printf("initial warnings: %s", strings.Join(first.Warnings, "; "))
	}

	var cache atomic.Value
	cache.Store(first)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(*interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				cache.Store(sampler.Snapshot(ctx))
			}
		}
	}()

	mux := http.NewServeMux()
	mux.Handle("/", webui.Handler())

	snapshot := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		snap, ok := cache.Load().(telemetry.Snapshot)
		if !ok {
			http.Error(w, `{"error":"no snapshot yet"}`, http.StatusServiceUnavailable)
			return
		}
		if err := json.NewEncoder(w).Encode(snap); err != nil {
			log.Printf("encode snapshot: %v", err)
		}
	}
	mux.HandleFunc("/api/snapshot", snapshot)

	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		snap, _ := cache.Load().(telemetry.Snapshot)
		age := time.Since(snap.GeneratedAt).Seconds()
		fmt.Fprintf(w, `{"ok":true,"snapshot_age_seconds":%.1f,"version":%q}`+"\n", age, version)
	})

	srv := &http.Server{
		Addr:              *listen,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
	}

	go func() {
		log.Printf("mf971v-dashboard %s listening on http://%s (router %s, interval %s)", version, *listen, *router, *interval)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %v", err)
		}
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
	wg.Wait()
	log.Printf("stopped")
}
