//go:build !darwin && !linux

package collector

import (
	"context"

	"github.com/dandisetiawanco/InfraOS/pkg/contracts"
)

func collectPlatformOptional(_ context.Context) contracts.OptionalMetrics {
	return contracts.OptionalMetrics{}
}
