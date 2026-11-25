package collector

import (
	"context"
	"math"
	"net"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/dandisetiawanco/InfraOS/pkg/contracts"
	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/load"
	"github.com/shirou/gopsutil/v4/mem"
	gnet "github.com/shirou/gopsutil/v4/net"
	"github.com/shirou/gopsutil/v4/process"
)

type Collector interface {
	Collect(ctx context.Context) (contracts.HostMetricPayload, error)
}

type netSample struct {
	rxBytes uint64
	txBytes uint64
	at      time.Time
}

type SystemCollector struct {
	agentVersion string
	mu           sync.Mutex
	prevTotal    *netSample
	prevByIface  map[string]netSample
}

func NewSystemCollector(agentVersion string) *SystemCollector {
	if agentVersion == "" {
		agentVersion = "0.1.0"
	}
	return &SystemCollector{
		agentVersion: agentVersion,
		prevByIface:  make(map[string]netSample),
	}
}

func (c *SystemCollector) Collect(ctx context.Context) (contracts.HostMetricPayload, error) {
	now := time.Now().UTC()
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "unknown-host"
	}

	info, _ := host.InfoWithContext(ctx)
	platform := runtime.GOOS
	platformVersion := ""
	kernelVersion := ""
	var uptimeSec uint64
	if info != nil {
		if info.Hostname != "" {
			hostname = info.Hostname
		}
		if info.Platform != "" {
			platform = info.Platform
		}
		platformVersion = info.PlatformVersion
		kernelVersion = info.KernelVersion
		uptimeSec = info.Uptime
	}

	cpuPercents, _ := cpu.PercentWithContext(ctx, 250*time.Millisecond, false)
	var cpuPct float64
	if len(cpuPercents) > 0 {
		cpuPct = round2(clampPercent(cpuPercents[0]))
	}
	cores, _ := cpu.CountsWithContext(ctx, true)
	if cores <= 0 {
		cores = runtime.NumCPU()
	}

	var load1, load5, load15 float64
	if avg, err := load.AvgWithContext(ctx); err == nil && avg != nil {
		load1 = round2(avg.Load1)
		load5 = round2(avg.Load5)
		load15 = round2(avg.Load15)
	}

	var memTotal, memUsed, memAvail uint64
	var memPct float64
	if vm, err := mem.VirtualMemoryWithContext(ctx); err == nil && vm != nil {
		memTotal = vm.Total
		memUsed = vm.Used
		memAvail = vm.Available
		memPct = round2(clampPercent(vm.UsedPercent))
	}

	filesystems, diskTotal, diskUsed, diskPct := collectFilesystems(ctx)
	ifaces, totalRx, totalTx, totalRxBps, totalTxBps := c.collectNetwork(ctx, now)

	optional := collectPlatformOptional(ctx)
	if pids, err := process.PidsWithContext(ctx); err == nil && len(pids) > 0 {
		optional.Processes = &contracts.ProcessSummary{TotalCount: len(pids)}
	}

	return contracts.HostMetricPayload{
		RecordedAt:      now,
		Hostname:        hostname,
		OS:              runtime.GOOS,
		Platform:        platform,
		PlatformVersion: platformVersion,
		Arch:            runtime.GOARCH,
		KernelVersion:   kernelVersion,
		IPAddress:       detectPrimaryIP(),
		AgentVersion:    c.agentVersion,
		CPUPercent:      cpuPct,
		CPUCores:        cores,
		Load1m:          load1,
		Load5m:          load5,
		Load15m:         load15,
		MemTotalBytes:   memTotal,
		MemUsedBytes:    memUsed,
		MemAvailBytes:   memAvail,
		MemPercent:      memPct,
		DiskTotalBytes:  diskTotal,
		DiskUsedBytes:   diskUsed,
		DiskPercent:     diskPct,
		NetRxBytes:      totalRx,
		NetTxBytes:      totalTx,
		NetRxBps:        totalRxBps,
		NetTxBps:        totalTxBps,
		UptimeSeconds:   uptimeSec,
		Filesystems:     filesystems,
		Interfaces:      ifaces,
		Optional:        optional,
	}, nil
}

func collectFilesystems(ctx context.Context) ([]contracts.FilesystemMetric, uint64, uint64, float64) {
	parts, err := disk.PartitionsWithContext(ctx, false)
	if err != nil {
		return []contracts.FilesystemMetric{}, 0, 0, 0
	}
	seenMount := make(map[string]struct{})
	seenDevice := make(map[string]struct{})
	out := make([]contracts.FilesystemMetric, 0, len(parts))

	var rootTotal, rootUsed uint64
	var rootPct float64
	hasRoot := false

	for _, p := range parts {
		if _, dup := seenMount[p.Mountpoint]; dup {
			continue
		}
		if strings.HasPrefix(p.Mountpoint, "/System/Volumes/") && p.Mountpoint != "/System/Volumes/Data" {
			continue
		}
		usage, err := disk.UsageWithContext(ctx, p.Mountpoint)
		if err != nil || usage == nil || usage.Total == 0 {
			continue
		}
		seenMount[p.Mountpoint] = struct{}{}
		entry := contracts.FilesystemMetric{
			MountPoint:  p.Mountpoint,
			Device:      p.Device,
			FSType:      p.Fstype,
			TotalBytes:  usage.Total,
			UsedBytes:   usage.Used,
			FreeBytes:   usage.Free,
			UsedPercent: round2(clampPercent(usage.UsedPercent)),
		}
		out = append(out, entry)

		if p.Mountpoint == "/" || p.Mountpoint == "/System/Volumes/Data" {
			// Prefer /System/Volumes/Data on macOS APFS when present so capacity matches user storage.
			if !hasRoot || p.Mountpoint == "/System/Volumes/Data" {
				rootTotal = usage.Total
				rootUsed = usage.Used
				rootPct = entry.UsedPercent
				hasRoot = true
			}
		}
		seenDevice[p.Device] = struct{}{}
	}

	if !hasRoot && len(out) > 0 {
		rootTotal = out[0].TotalBytes
		rootUsed = out[0].UsedBytes
		rootPct = out[0].UsedPercent
	}
	return out, rootTotal, rootUsed, rootPct
}

func (c *SystemCollector) collectNetwork(ctx context.Context, now time.Time) ([]contracts.NetworkInterfaceMetric, uint64, uint64, float64, float64) {
	counters, err := gnet.IOCountersWithContext(ctx, true)
	if err != nil {
		return []contracts.NetworkInterfaceMetric{}, 0, 0, 0, 0
	}

	addrMap := make(map[string][]string)
	if sysIfaces, err := net.Interfaces(); err == nil {
		for _, iface := range sysIfaces {
			addrs, err := iface.Addrs()
			if err != nil {
				continue
			}
			for _, a := range addrs {
				addrMap[iface.Name] = append(addrMap[iface.Name], a.String())
			}
		}
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	var totalRx, totalTx uint64
	out := make([]contracts.NetworkInterfaceMetric, 0, len(counters))
	for _, ioStat := range counters {
		if isVirtualNoiseInterface(ioStat.Name) {
			continue
		}
		if ioStat.BytesRecv == 0 && ioStat.BytesSent == 0 && len(addrMap[ioStat.Name]) == 0 {
			continue
		}
		totalRx += ioStat.BytesRecv
		totalTx += ioStat.BytesSent

		var rxBps, txBps float64
		if prev, ok := c.prevByIface[ioStat.Name]; ok {
			rxBps, txBps = ComputeThroughput(prev.rxBytes, prev.txBytes, prev.at, ioStat.BytesRecv, ioStat.BytesSent, now)
		}
		c.prevByIface[ioStat.Name] = netSample{rxBytes: ioStat.BytesRecv, txBytes: ioStat.BytesSent, at: now}

		out = append(out, contracts.NetworkInterfaceMetric{
			Name:      ioStat.Name,
			Addresses: addrMap[ioStat.Name],
			RxBytes:   ioStat.BytesRecv,
			TxBytes:   ioStat.BytesSent,
			RxBps:     rxBps,
			TxBps:     txBps,
		})
	}

	var totalRxBps, totalTxBps float64
	if c.prevTotal != nil {
		totalRxBps, totalTxBps = ComputeThroughput(c.prevTotal.rxBytes, c.prevTotal.txBytes, c.prevTotal.at, totalRx, totalTx, now)
	}
	c.prevTotal = &netSample{rxBytes: totalRx, txBytes: totalTx, at: now}

	return out, totalRx, totalTx, totalRxBps, totalTxBps
}

// ComputeThroughput calculates bytes per second from counter deltas and guards against counter resets.
func ComputeThroughput(prevRx, prevTx uint64, prevAt time.Time, currRx, currTx uint64, currAt time.Time) (float64, float64) {
	elapsed := currAt.Sub(prevAt).Seconds()
	if elapsed <= 0 {
		return 0, 0
	}
	var deltaRx, deltaTx uint64
	if currRx >= prevRx {
		deltaRx = currRx - prevRx
	}
	if currTx >= prevTx {
		deltaTx = currTx - prevTx
	}
	return round2(float64(deltaRx) / elapsed), round2(float64(deltaTx) / elapsed)
}

func isVirtualNoiseInterface(name string) bool {
	lower := strings.ToLower(name)
	return lower == "lo" || lower == "lo0" || strings.HasPrefix(lower, "gif") || strings.HasPrefix(lower, "stf") || strings.HasPrefix(lower, "awdl") || strings.HasPrefix(lower, "llw")
}

func detectPrimaryIP() string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return "127.0.0.1"
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			ipNet, ok := a.(*net.IPNet)
			if !ok || ipNet.IP == nil || ipNet.IP.IsLoopback() {
				continue
			}
			if v4 := ipNet.IP.To4(); v4 != nil {
				return v4.String()
			}
		}
	}
	return "127.0.0.1"
}

func clampPercent(v float64) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return v
}

func round2(v float64) float64 {
	return math.Round(v*100) / 100
}
