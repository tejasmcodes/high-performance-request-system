package main

import (
	"flag"
	"log"
	"net/http"

	"github.com/tejasmcodes/high-performance-request-system/internal/api"
	"github.com/tejasmcodes/high-performance-request-system/internal/benchmark"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:8082", "API listen address")
	scenarioDir := flag.String("scenario-dir", "bench/scenarios", "directory containing benchmark scenarios")
	corsOrigin := flag.String("cors-origin", "http://localhost:5173", "allowed development frontend origin")
	flag.Parse()

	handler := api.New(*scenarioDir, *corsOrigin, benchmark.Service{})
	log.Printf("benchmark API listening on %s", *addr)
	if err := http.ListenAndServe(*addr, handler); err != nil {
		log.Fatal(err)
	}
}
