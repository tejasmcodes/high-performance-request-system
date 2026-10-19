package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/tejasmcodes/high-performance-request-system/internal/metrics"
)

func main() {
	port := flag.Int("port", 8081, "port for the backend server")
	id := flag.String("id", "backend-1", "identifier for the backend server")

	flag.Parse()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sampler := metrics.NewCPUSampler()
	go sampler.Run(ctx, metrics.DefaultCPUSampleInterval)

	mux := http.NewServeMux()

	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		delay, err := strconv.Atoi(r.URL.Query().Get("delay"))

		if err == nil && delay > 0 {
			time.Sleep(time.Duration(delay) * time.Second)
		}
		fmt.Fprintf(w, "Hello from %s\n", *id)
	})

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "OK")
	})

	mux.HandleFunc("GET /metrics/cpu", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if err := json.NewEncoder(w).Encode(map[string]float64{
			"cpu_utilization": sampler.Value(),
		}); err != nil {
			log.Printf("encode CPU metrics: %v", err)
		}
	})

	addr := fmt.Sprintf(":%d", *port)

	log.Printf("%s listening on %s", *id, addr)

	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err)
	}
}
