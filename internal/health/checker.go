package health

import (
	"context"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/tejasmcodes/high-performance-request-system/internal/models"
)

type HealthChecker struct {
	servers []*models.Server
	client  *http.Client
}

func NewHealthChecker(servers []*models.Server) *HealthChecker {
	return &HealthChecker{
		servers: servers,
		client: &http.Client{
			Timeout: 2 * time.Second,
		},
	}
}

func (hc *HealthChecker) checkServer(server *models.Server) {
	healthURL := server.URL + "/health"

	wasHealthy := server.IsHealthy()

	resp, err := hc.client.Get(healthURL)
	if err != nil {
		server.SetHealthy(false)
		if wasHealthy{
			log.Printf("%s is unhealthy: %v", server.ID, err)
		}
		return
	}

	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		server.SetHealthy(true)
		if !wasHealthy{
			log.Printf("%s recovered and is healthy", server.ID)
		}
		return
	}

	server.SetHealthy(false)
	if wasHealthy{
		log.Printf("%s is unhealthy: status %d", server.ID, resp.StatusCode)
	}
	
}

func (hc *HealthChecker) CheckAll() {
	var wg sync.WaitGroup

	for _, server := range hc.servers {
		wg.Add(1)

		go func(server *models.Server) {
			defer wg.Done()

			hc.checkServer(server)
		}(server)
	}
	wg.Wait()
}

func (hc *HealthChecker) Start(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			hc.CheckAll()

		case <-ctx.Done():
			return
		}
	}
}
