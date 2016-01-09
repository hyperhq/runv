// +build linux,with_libvirt

package libvirt

import (
	"encoding/xml"

	"fmt"
	libvirtgo "github.com/alexzorin/libvirt-go"
	"github.com/golang/glog"
	"github.com/hyperhq/runv/hypervisor"
	"github.com/hyperhq/runv/hypervisor/types"
	"time"
)

/*
#cgo LDFLAGS: -lvirt
#include <libvirt/libvirt.h>
#include <libvirt/virterror.h>
#include <stdlib.h>
*/
import "C"

const (
	VIR_DOMAIN_MEMORY_STAT_SWAP_IN        = C.VIR_DOMAIN_MEMORY_STAT_SWAP_IN
	VIR_DOMAIN_MEMORY_STAT_SWAP_OUT       = C.VIR_DOMAIN_MEMORY_STAT_SWAP_OUT
	VIR_DOMAIN_MEMORY_STAT_MAJOR_FAULT    = C.VIR_DOMAIN_MEMORY_STAT_MAJOR_FAULT
	VIR_DOMAIN_MEMORY_STAT_MINOR_FAULT    = C.VIR_DOMAIN_MEMORY_STAT_MINOR_FAULT
	VIR_DOMAIN_MEMORY_STAT_UNUSED         = C.VIR_DOMAIN_MEMORY_STAT_UNUSED
	VIR_DOMAIN_MEMORY_STAT_AVAILABLE      = C.VIR_DOMAIN_MEMORY_STAT_AVAILABLE
	VIR_DOMAIN_MEMORY_STAT_ACTUAL_BALLOON = C.VIR_DOMAIN_MEMORY_STAT_ACTUAL_BALLOON
	VIR_DOMAIN_MEMORY_STAT_RSS            = C.VIR_DOMAIN_MEMORY_STAT_RSS
	VIR_DOMAIN_MEMORY_STAT_NR             = C.VIR_DOMAIN_MEMORY_STAT_NR

	VIR_DOMAIN_CPU_STATS_CPUTIME    = C.VIR_DOMAIN_CPU_STATS_CPUTIME
	VIR_DOMAIN_CPU_STATS_USERTIME   = C.VIR_DOMAIN_CPU_STATS_USERTIME
	VIR_DOMAIN_CPU_STATS_SYSTEMTIME = C.VIR_DOMAIN_CPU_STATS_SYSTEMTIME
)

type VirDomain struct {
	Name    string    `xml:"name"`
	UUID    string    `xml:"uuid"`
	Memory  string    `xml:"memory"`
	Devices VirDevice `xml:"devices"`
}

type VirDevice struct {
	Disks      []VirDisk      `xml:"disk"`
	Interfaces []VirInterface `xml:"interface"`
}

// TODO: Support filesystem and rbd stats
type VirDisk struct {
	Type   string        `xml:"type,attr"`
	Source VirDiskSource `xml:"source"`
	Target VirDiskTarget `xml:"target"`
}

type VirDiskSource struct {
	File string `xml:"file,attr"`
}

type VirDiskTarget struct {
	Dev string `xml:"dev,attr"`
}

type VirInterface struct {
	Type   string             `xml:"type,attr"`
	Device VirInterfaceTarget `xml:"target"`
	Mac    VirInterfaceMac    `xml:"mac"`
}

type VirInterfaceTarget struct {
	Dev string `xml:"dev,attr"`
}

type VirInterfaceMac struct {
	Address string `xml:"mac,attr"`
}

func GetCPUStats(domain *libvirtgo.VirDomain) (types.CpuStats, error) {
	stats := types.CpuStats{
		Usage: types.CpuUsage{
			PerCpu: make([]uint64, 0, 1),
		},
	}

	// Get the number of cpus available to query from the host perspective,
	ncpus, err := domain.GetCPUStats(nil, 0, 0, 0, 0)
	if err != nil {
		return stats, err
	}

	// Get how many statistics are available for the given @start_cpu.
	nparams, err := domain.GetCPUStats(nil, 0, 0, 1, 0)
	if err != nil {
		return stats, err
	}

	// Query per-cpu stats
	var perCPUStats libvirtgo.VirTypedParameters
	_, err = domain.GetCPUStats(&perCPUStats, nparams, 0, uint32(ncpus), 0)
	if err != nil {
		return stats, err
	}
	if len(perCPUStats) == 0 {
		return stats, fmt.Errorf("Can't get per-cpu stats")
	}
	for _, stat := range perCPUStats {
		stats.Usage.PerCpu = append(stats.Usage.PerCpu, stat.Value.(uint64))
	}
	glog.V(4).Infof("Get per-cpu stats: %v", perCPUStats)

	// Query total stats
	var cpuStats libvirtgo.VirTypedParameters
	nparams, err = domain.GetCPUStats(nil, 0, -1, 1, 0)
	if err != nil {
		return stats, err
	}
	_, err = domain.GetCPUStats(&cpuStats, nparams, -1, 1, 0)
	if err != nil {
		return stats, err
	}
	for _, stat := range cpuStats {
		switch stat.Name {
		case VIR_DOMAIN_CPU_STATS_CPUTIME:
			stats.Usage.Total = stat.Value.(uint64)
		case VIR_DOMAIN_CPU_STATS_USERTIME:
			stats.Usage.User = stat.Value.(uint64)
		case VIR_DOMAIN_CPU_STATS_SYSTEMTIME:
			stats.Usage.System = stat.Value.(uint64)
		}
	}
	glog.V(4).Infof("Get total cpu stats: %v", cpuStats)

	return stats, nil
}

func GetMemoryStats(domain *libvirtgo.VirDomain) (types.MemoryStats, error) {
	stats := types.MemoryStats{}

	memStats, err := domain.MemoryStats(VIR_DOMAIN_MEMORY_STAT_NR, 0)
	if err != nil {
		return stats, err
	}

	var unused, available uint64
	for _, stat := range memStats {
		if stat.Tag == VIR_DOMAIN_MEMORY_STAT_UNUSED {
			unused = stat.Val
		} else if stat.Tag == VIR_DOMAIN_MEMORY_STAT_AVAILABLE {
			available = stat.Val
		}
	}

	if available > unused {
		stats.Usage = (available - unused) * 1024
	}

	return stats, nil
}

func GetNetworkStats(domain *libvirtgo.VirDomain, virDomain *VirDomain) (types.NetworkStats, error) {
	stats := types.NetworkStats{
		Interfaces: make([]types.InterfaceStats, 0, 1),
	}

	for _, iface := range virDomain.Devices.Interfaces {
		ifaceStats, err := domain.InterfaceStats(iface.Device.Dev)
		if err != nil {
			return stats, err
		}

		stats.Interfaces = append(stats.Interfaces, types.InterfaceStats{
			Name:      iface.Device.Dev,
			RxBytes:   uint64(ifaceStats.TxBytes),
			RxPackets: uint64(ifaceStats.RxPackets),
			RxErrors:  uint64(ifaceStats.RxErrs),
			RxDropped: uint64(ifaceStats.RxDrop),
			TxBytes:   uint64(ifaceStats.TxBytes),
			TxPackets: uint64(ifaceStats.TxPackets),
			TxErrors:  uint64(ifaceStats.TxErrs),
			TxDropped: uint64(ifaceStats.TxDrop),
		})

		stats.RxBytes = stats.RxBytes + uint64(ifaceStats.TxBytes)
		stats.RxPackets = stats.RxPackets + uint64(ifaceStats.RxPackets)
		stats.RxErrors = stats.RxErrors + uint64(ifaceStats.RxErrs)
		stats.RxDropped = stats.RxDropped + uint64(ifaceStats.RxDrop)
		stats.TxBytes = stats.TxBytes + uint64(ifaceStats.TxBytes)
		stats.TxPackets = stats.TxPackets + uint64(ifaceStats.TxPackets)
		stats.TxErrors = stats.TxErrors + uint64(ifaceStats.TxErrs)
		stats.TxDropped = stats.TxDropped + uint64(ifaceStats.TxDrop)
	}

	return stats, nil
}

func GetBlockStats(domain *libvirtgo.VirDomain, virDomain *VirDomain) (types.BlockStats, error) {
	stats := types.BlockStats{
		IoStats: make([]types.DiskStats, 0, 1),
	}

	for _, blk := range virDomain.Devices.Disks {
		blkStats, err := domain.BlockStats(blk.Target.Dev)
		if err != nil {
			return stats, err
		}

		stats.IoStats = append(stats.IoStats, types.DiskStats{
			DiskName:   blk.Target.Dev,
			RdRequests: uint64(blkStats.RdReq),
			WrRequests: uint64(blkStats.WrReq),
			RdBytes:    uint64(blkStats.RdBytes),
			WrBytes:    uint64(blkStats.WrBytes),
		})

		stats.RdBytes = stats.RdBytes + uint64(blkStats.RdBytes)
		stats.RdRequests = stats.RdRequests + uint64(blkStats.RdReq)
		stats.WrBytes = stats.WrBytes + uint64(blkStats.WrBytes)
		stats.WrRequests = stats.WrRequests + uint64(blkStats.WrReq)
	}

	return stats, nil
}

func (lc *LibvirtContext) Stats(ctx *hypervisor.VmContext) (*types.PodStats, error) {
	xmlDesc, err := lc.domain.GetXMLDesc(0)
	if err != nil {
		return nil, err
	}
	glog.V(4).Infof("XML description for domain is %s", xmlDesc)

	var virDomain VirDomain
	err = xml.Unmarshal([]byte(xmlDesc), &virDomain)
	if err != nil {
		return nil, err
	}
	glog.V(4).Infof("Get domain description: %v", virDomain)

	cpuStats, err := GetCPUStats(lc.domain)
	if err != nil {
		return nil, err
	}

	memoryStats, err := GetMemoryStats(lc.domain)
	if err != nil {
		return nil, err
	}

	networkStats, err := GetNetworkStats(lc.domain, &virDomain)
	if err != nil {
		return nil, err
	}

	blockStats, err := GetBlockStats(lc.domain, &virDomain)
	if err != nil {
		return nil, err
	}

	return &types.PodStats{
		Cpu:       cpuStats,
		Memory:    memoryStats,
		Disk:      blockStats,
		Network:   networkStats,
		Timestamp: time.Now(),
	}, nil
}
