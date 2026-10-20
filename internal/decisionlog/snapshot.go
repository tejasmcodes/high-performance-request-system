package decisionlog

import (
	"time"

	"github.com/tejasmcodes/high-performance-request-system/internal/models"
)

type CandidateSnapshot struct {
	BackendID           string    `json:"backend_id"`
	Healthy             bool      `json:"healthy"`
	Weight              int       `json:"weight"`
	ActiveConnections   int       `json:"active_connections"`
	ResponseTimeEMAMs   float64   `json:"response_time_ema_ms"`
	ResponseTimeSamples int64     `json:"response_time_samples"`
	CPUUtilization      float64   `json:"cpu_utilization"`
	CPUUpdatedAt        time.Time `json:"cpu_updated_at"`
	CPUAgeMs            int64     `json:"cpu_age_ms"`
}

func SnapshotServer(server *models.Server, now time.Time) CandidateSnapshot {
	if server == nil {
		return CandidateSnapshot{}
	}

	cpuUpdatedAt := server.CPUUpdatedAt()

	return CandidateSnapshot{
		BackendID:           server.ID,
		Healthy:             server.IsHealthy(),
		Weight:              server.Weight,
		ActiveConnections:   server.ActiveConnections(),
		ResponseTimeEMAMs:   server.ResponseTime(),
		ResponseTimeSamples: server.ResponseTimeSamples(),
		CPUUtilization:      server.CPUUtilization(),
		CPUUpdatedAt:        cpuUpdatedAt,
		CPUAgeMs:            cpuAgeMs(cpuUpdatedAt, now),
	}
}

func SnapshotServers(servers []*models.Server, now time.Time) []CandidateSnapshot {
	snapshots := make([]CandidateSnapshot, 0, len(servers))

	for _, server := range servers {
		if server == nil {
			continue
		}

		snapshots = append(snapshots, SnapshotServer(server, now))
	}

	return snapshots
}

func cpuAgeMs(updatedAt, now time.Time) int64 {
	if updatedAt.IsZero() || now.IsZero() {
		return 0
	}

	age := now.Sub(updatedAt)
	if age <= 0 {
		return 0
	}

	return age.Milliseconds()
}
