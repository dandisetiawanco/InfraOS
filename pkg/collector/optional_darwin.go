//go:build darwin

package collector

import (
	"context"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/dandisetiawanco/InfraOS/pkg/contracts"
)

var pmsetBatteryRegex = regexp.MustCompile(`(\d+)%;\s*([^;]+);`)

func collectPlatformOptional(ctx context.Context) contracts.OptionalMetrics {
	var opt contracts.OptionalMetrics
	subCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	out, err := exec.CommandContext(subCtx, "/usr/bin/pmset", "-g", "batt").Output()
	if err != nil {
		return opt
	}
	text := string(out)
	match := pmsetBatteryRegex.FindStringSubmatch(text)
	if len(match) < 3 {
		return opt
	}
	pct, err := strconv.ParseFloat(match[1], 64)
	if err != nil {
		return opt
	}
	source := "battery"
	if strings.Contains(text, "AC Power") {
		source = "ac_power"
	}
	opt.Battery = &contracts.BatteryMetric{
		Percent: pct,
		State:   strings.TrimSpace(match[2]),
		Source:  source,
	}
	return opt
}
