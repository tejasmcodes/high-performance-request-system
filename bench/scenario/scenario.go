package scenario

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
)

type Backend struct {
	ID        string `json:"id"`
	Port      int    `json:"port"`
	Weight    int    `json:"weight"`
	LatencyMS int    `json:"latency_ms"`
}

type Phase struct {
	AtS        int    `json:"at_s"`
	Action     string `json:"action"`
	Backend    string `json:"backend"`
	LatencyMS  int    `json:"latency_ms,omitempty"`
	CPUWorkers int    `json:"cpu_workers,omitempty"`
}

type Load struct {
	WarmupS       int     `json:"warmup_s"`
	DurationS     int     `json:"duration_s"`
	Concurrency   int     `json:"concurrency"`
	RatePerSecond float64 `json:"rate_per_second"`
}

type Scenario struct {
	Name        string    `json:"name"`
	Description string    `json:"description"`
	LBAddr      string    `json:"lb_addr"`
	Backends    []Backend `json:"backends"`
	Phases      []Phase   `json:"phases"`
	Load        Load      `json:"load"`
	Strategies  []string  `json:"strategies"`
	Repeats     int       `json:"repeats"`
}

func LoadScenario(path string) (*Scenario, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var sc Scenario
	if err := json.Unmarshal(data, &sc); err != nil {
		return nil, fmt.Errorf("parse scenario %s: %w", path, err)
	}

	sc.applyDefaults()

	if err := sc.validate(); err != nil {
		return nil, fmt.Errorf("scenario %s: %w", path, err)
	}

	sort.Slice(sc.Phases, func(i, j int) bool {
		return sc.Phases[i].AtS < sc.Phases[j].AtS
	})

	return &sc, nil
}

func (s *Scenario) applyDefaults() {
	if s.LBAddr == "" {
		s.LBAddr = ":18080"
	}
	if s.Load.Concurrency <= 0 {
		s.Load.Concurrency = 32
	}
	if s.Repeats <= 0 {
		s.Repeats = 1
	}
	if len(s.Strategies) == 0 {
		s.Strategies = []string{"round-robin", "least-connections", "weighted-round-robin", "adaptive"}
	}
}

func (s *Scenario) validate() error {
	if s.Name == "" {
		return fmt.Errorf("name is required")
	}
	if len(s.Backends) == 0 {
		return fmt.Errorf("at least one backend is required")
	}
	if s.Load.DurationS <= 0 {
		return fmt.Errorf("load.duration_s must be positive")
	}
	if s.Load.WarmupS < 0 {
		return fmt.Errorf("load.warmup_s must not be negative")
	}

	seen := make(map[string]bool, len(s.Backends))
	for _, backend := range s.Backends {
		if backend.ID == "" {
			return fmt.Errorf("backend id is required")
		}
		if backend.Port <= 0 || backend.Port > 65535 {
			return fmt.Errorf("backend %s has invalid port %d", backend.ID, backend.Port)
		}
		if seen[backend.ID] {
			return fmt.Errorf("duplicate backend id %q", backend.ID)
		}
		seen[backend.ID] = true
	}

	for _, phase := range s.Phases {
		if phase.AtS < 0 {
			return fmt.Errorf("phase at_s must not be negative")
		}
		switch phase.Action {
		case "none":
		case "set_latency", "set_cpu":
			if phase.Backend == "" {
				return fmt.Errorf("phase %q at %ds requires a backend", phase.Action, phase.AtS)
			}
			if !seen[phase.Backend] {
				return fmt.Errorf("phase references unknown backend %q", phase.Backend)
			}
		default:
			return fmt.Errorf("unknown phase action %q", phase.Action)
		}
	}

	return nil
}

func (s *Scenario) IDs() []string {
	ids := make([]string, len(s.Backends))
	for i, backend := range s.Backends {
		ids[i] = backend.ID
	}
	return ids
}

func (s *Scenario) URLs() []string {
	urls := make([]string, len(s.Backends))
	for i, backend := range s.Backends {
		urls[i] = fmt.Sprintf("http://127.0.0.1:%d", backend.Port)
	}
	return urls
}

func (s *Scenario) BackendURL(id string) (string, bool) {
	for _, backend := range s.Backends {
		if backend.ID == id {
			return fmt.Sprintf("http://127.0.0.1:%d", backend.Port), true
		}
	}
	return "", false
}

func JoinInts(values []int) string {
	parts := make([]string, len(values))
	for i, value := range values {
		parts[i] = fmt.Sprintf("%d", value)
	}
	return strings.Join(parts, ",")
}
