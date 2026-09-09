package metrics

import (
	"bufio"
	"os"
	"strconv"
	"strings"
	"time"
)

func readCPU() (idle, total uint64, err error) {
	file, err := os.Open("/proc/stat")
	if err != nil {
		return 0, 0, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)

	if !scanner.Scan() {
		return 0, 0, scanner.Err()
	}

	fields := strings.Fields(scanner.Text())

	// cpu user nice system idle iowait irq softirq steal ...
	if len(fields) < 5 || fields[0] != "cpu" {
		return 0, 0, nil
	}

	for _, field := range fields[1:] {
		value, err := strconv.ParseUint(field, 10, 64)
		if err != nil {
			return 0, 0, err
		}
		total += value
	}

	idle, err = strconv.ParseUint(fields[4], 10, 64)
	if err != nil {
		return 0, 0, err
	}

	return idle, total, nil
}

func CPUUtilization() (float64, error) {
	idle1, total1, err := readCPU()
	if err != nil {
		return 0, err
	}

	time.Sleep(500 * time.Millisecond)

	idle2, total2, err := readCPU()
	if err != nil {
		return 0, err
	}

	totalDelta := total2 - total1
	idleDelta := idle2 - idle1

	if totalDelta == 0 {
		return 0, nil
	}

	return (1 - float64(idleDelta)/float64(totalDelta)) * 100, nil
}