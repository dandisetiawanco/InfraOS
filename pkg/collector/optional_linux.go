//go:build linux

package collector

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/dandisetiawanco/InfraOS/pkg/contracts"
)

func collectPlatformOptional(_ context.Context) contracts.OptionalMetrics {
	var opt contracts.OptionalMetrics

	if matches, err := filepath.Glob("/sys/class/thermal/thermal_zone*/temp"); err == nil {
		for _, p := range matches {
			raw, err := os.ReadFile(p)
			if err != nil {
				continue
			}
			milliC, err := strconv.ParseFloat(strings.TrimSpace(string(raw)), 64)
			if err != nil || milliC <= 0 {
				continue
			}
			c := round2(milliC / 1000.0)
			opt.CPUTemperatureC = &c
			break
		}
	}

	if matches, err := filepath.Glob("/sys/class/power_supply/BAT*/capacity"); err == nil && len(matches) > 0 {
		dir := filepath.Dir(matches[0])
		if capRaw, err := os.ReadFile(matches[0]); err == nil {
			if pct, err := strconv.ParseFloat(strings.TrimSpace(string(capRaw)), 64); err == nil {
				state := "unknown"
				if stRaw, err := os.ReadFile(filepath.Join(dir, "status")); err == nil {
					state = strings.ToLower(strings.TrimSpace(string(stRaw)))
				}
				opt.Battery = &contracts.BatteryMetric{Percent: pct, State: state}
			}
		}
	}
	return opt
}
