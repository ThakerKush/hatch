package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ThakerKush/Hatch/internal/store"
)

type vmMetricsResponse struct {
	VMID          string         `json:"vm_id"`
	State         string         `json:"state"`
	VCPUCount     int            `json:"vcpu_count"`
	MemMib        int            `json:"mem_mib"`
	UptimeSeconds int64          `json:"uptime_seconds"`
	Net           netMetrics     `json:"net"`
	Block         blockMetrics   `json:"block"`
	VCPU          vcpuMetrics    `json:"vcpu"`
	Raw           map[string]any `json:"raw"`
}

type netMetrics struct {
	RXBytes   int64 `json:"rx_bytes"`
	TXBytes   int64 `json:"tx_bytes"`
	RXPackets int64 `json:"rx_packets"`
	TXPackets int64 `json:"tx_packets"`
}

type blockMetrics struct {
	ReadBytes  int64 `json:"read_bytes"`
	WriteBytes int64 `json:"write_bytes"`
}

type vcpuMetrics struct {
	CPUTimeMicros      int64   `json:"cpu_time_micros"`
	UtilizationPercent float64 `json:"utilization_percent"`
}

func readLatestVMMetrics(vm store.VM) (*vmMetricsResponse, error) {
	metricsPath := filepath.Join(vm.WorkDir, "firecracker.metrics")
	raw, err := readLastJSONLine(metricsPath)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	uptime := int64(0)
	if vm.CreatedAt.Before(now) {
		uptime = int64(now.Sub(vm.CreatedAt).Seconds())
	}

	cpuTimeMicros := sumMetricsValues(raw, "cpu_time_us", "cpu_time_usec", "cpu_time_micros")
	utilization := 0.0
	if vm.VCPUCount > 0 && uptime > 0 && cpuTimeMicros > 0 {
		totalAvailableMicros := float64(vm.VCPUCount) * float64(uptime) * 1_000_000
		utilization = (float64(cpuTimeMicros) / totalAvailableMicros) * 100
		utilization = math.Max(0, math.Min(utilization, 100))
	}

	return &vmMetricsResponse{
		VMID:          vm.ID,
		State:         vm.State,
		VCPUCount:     vm.VCPUCount,
		MemMib:        vm.MemMib,
		UptimeSeconds: uptime,
		Net: netMetrics{
			RXBytes:   sumMetricsValues(raw, "rx_bytes_count", "rx_bytes"),
			TXBytes:   sumMetricsValues(raw, "tx_bytes_count", "tx_bytes"),
			RXPackets: sumMetricsValues(raw, "rx_packets_count", "rx_packets"),
			TXPackets: sumMetricsValues(raw, "tx_packets_count", "tx_packets"),
		},
		Block: blockMetrics{
			ReadBytes:  sumMetricsValues(raw, "read_bytes", "read_bytes_count"),
			WriteBytes: sumMetricsValues(raw, "write_bytes", "write_bytes_count"),
		},
		VCPU: vcpuMetrics{
			CPUTimeMicros:      cpuTimeMicros,
			UtilizationPercent: utilization,
		},
		Raw: raw,
	}, nil
}

func readLastJSONLine(path string) (map[string]any, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read metrics file: %w", err)
	}

	lines := bytes.Split(content, []byte("\n"))
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(string(lines[i]))
		if line == "" {
			continue
		}
		var payload map[string]any
		decoder := json.NewDecoder(strings.NewReader(line))
		decoder.UseNumber()
		if err := decoder.Decode(&payload); err != nil {
			return nil, fmt.Errorf("parse metrics line: %w", err)
		}
		return payload, nil
	}

	return map[string]any{}, nil
}

func sumMetricsValues(payload map[string]any, keys ...string) int64 {
	var total int64
	var walk func(map[string]any)

	walk = func(node map[string]any) {
		for k, v := range node {
			lk := strings.ToLower(k)
			for _, key := range keys {
				if strings.Contains(lk, strings.ToLower(key)) {
					total += numberToInt64(v)
					break
				}
			}

			switch child := v.(type) {
			case map[string]any:
				walk(child)
			case []any:
				for _, entry := range child {
					if childMap, ok := entry.(map[string]any); ok {
						walk(childMap)
					}
				}
			}
		}
	}

	walk(payload)
	return total
}

func numberToInt64(value any) int64 {
	switch v := value.(type) {
	case json.Number:
		if i, err := v.Int64(); err == nil {
			return i
		}
		if f, err := v.Float64(); err == nil {
			return int64(f)
		}
	case float64:
		return int64(v)
	case float32:
		return int64(v)
	case int:
		return int64(v)
	case int64:
		return v
	case int32:
		return int64(v)
	case uint64:
		return int64(v)
	case uint32:
		return int64(v)
	}
	return 0
}
