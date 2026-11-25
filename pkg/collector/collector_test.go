package collector

import (
	"context"
	"testing"
	"time"
)

func TestComputeThroughputHandlesDeltaAndCounterResets(t *testing.T) {
	t0 := time.Date(2026, 11, 25, 10, 0, 0, 0, time.UTC)
	t1 := t0.Add(10 * time.Second)

	rx, tx := ComputeThroughput(1000, 2000, t0, 11000, 7000, t1)
	if rx != 1000 || tx != 500 {
		t.Fatalf("unexpected throughput: rx=%v tx=%v", rx, tx)
	}

	// Counter reset (curr < prev) must yield 0 instead of uint64 underflow.
	rxReset, txReset := ComputeThroughput(50000, 90000, t0, 120, 45, t1)
	if rxReset != 0 || txReset != 0 {
		t.Fatalf("expected 0 on counter reset, got rx=%v tx=%v", rxReset, txReset)
	}
}

func TestRealLocalSystemCollection(t *testing.T) {
	c := NewSystemCollector("0.1.0-test")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	sample, err := c.Collect(ctx)
	if err != nil {
		t.Fatalf("Collect failed: %v", err)
	}
	if sample.Hostname == "" {
		t.Error("expected non-empty hostname")
	}
	if sample.CPUCores < 1 {
		t.Errorf("expected at least 1 CPU core, got %d", sample.CPUCores)
	}
	if sample.MemTotalBytes == 0 {
		t.Error("expected non-zero total memory")
	}
	if sample.CPUPercent < 0 || sample.CPUPercent > 100 {
		t.Errorf("CPU percent out of bounds: %v", sample.CPUPercent)
	}
	if sample.MemPercent <= 0 || sample.MemPercent > 100 {
		t.Errorf("memory percent out of bounds: %v", sample.MemPercent)
	}
	if sample.DiskTotalBytes == 0 {
		t.Error("expected non-zero disk total bytes")
	}
}
