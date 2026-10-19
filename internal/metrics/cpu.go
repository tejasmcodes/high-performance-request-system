package metrics

import (
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const cpuSampleWindow = 500 * time.Millisecond

func readProcessCPUTime() (uint64, error) {
	data, err := os.ReadFile("/proc/self/schedstat")
	if err != nil {
		return 0, err
	}

	fields := strings.Fields(string(data))
	if len(fields) == 0 {
		return 0, fmt.Errorf("unexpected /proc/self/schedstat format")
	}

	cpuNanoseconds, err := strconv.ParseUint(fields[0], 10, 64)
	if err != nil {
		return 0, err
	}

	return cpuNanoseconds, nil
}

func ProcessCPUUtilization() (float64, error) {
	start := time.Now()

	before, err := readProcessCPUTime()
	if err != nil {
		return 0, err
	}

	time.Sleep(cpuSampleWindow)

	after, err := readProcessCPUTime()
	if err != nil {
		return 0, err
	}

	elapsed := time.Since(start).Nanoseconds()
	if elapsed <= 0 {
		return 0, nil
	}

	if after < before {
		return 0, fmt.Errorf("process CPU time went backwards")
	}

	cores := runtime.NumCPU()
	if cores <= 0 {
		cores = 1
	}

	return float64(after-before) / float64(elapsed) / float64(cores) * 100, nil
}
