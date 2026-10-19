package monitor

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"sync"
	"time"

	"github.com/tejasmcodes/high-performance-request-system/internal/models"
)

const (
	cpuMetricsPath         = "/metrics/cpu"
	defaultCollectInterval = 5 * time.Second
)

type cpuMetricsResponse struct {
	CPUUtilization float64 `json:"cpu_utilization"`
}

type CPUCollector struct {
	servers []*models.Server
	client  *http.Client
}

func NewCPUCollector(servers []*models.Server) *CPUCollector {
	return &CPUCollector{
		servers: servers,
		client: &http.Client{
			Timeout: 2 * time.Second,
		},
	}
}

func (c *CPUCollector) collect(ctx context.Context, server *models.Server) error {
	url := server.URL + cpuMetricsPath

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	var metrics cpuMetricsResponse
	if err := json.NewDecoder(resp.Body).Decode(&metrics); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	if math.IsNaN(metrics.CPUUtilization) || metrics.CPUUtilization < 0 || metrics.CPUUtilization > 100 {
		return fmt.Errorf("invalid cpu utilization %f", metrics.CPUUtilization)
	}

	server.SetCPUUtilization(metrics.CPUUtilization)

	return nil
}

func (c *CPUCollector) CollectAll(ctx context.Context) {
	var wg sync.WaitGroup

	for _, server := range c.servers {
		wg.Add(1)

		go func(server *models.Server) {
			defer wg.Done()

			if err := c.collect(ctx, server); err != nil {
				log.Printf("CPU collection failed for %s: %v", server.ID, err)
			}
		}(server)
	}

	wg.Wait()
}

func (c *CPUCollector) Start(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = defaultCollectInterval
	}

	c.CollectAll(ctx)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return

		case <-ticker.C:
			c.CollectAll(ctx)
		}
	}
}
