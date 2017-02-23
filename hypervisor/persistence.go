package hypervisor

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/golang/glog"
	"github.com/hyperhq/runv/api"
	hyperstartapi "github.com/hyperhq/runv/hyperstart/api/json"
	"github.com/hyperhq/runv/hypervisor/types"
	"github.com/hyperhq/runv/lib/utils"
)

const CURRENT_PERSIST_VERSION = 20170224

type PersistContainerInfo struct {
	Description *api.ContainerDescription
	Root        *PersistVolumeInfo
	Process     *hyperstartapi.Process
	Fsmap       []*hyperstartapi.FsmapDescriptor
	VmVolumes   []*hyperstartapi.VolumeDescriptor
}

type PersistVolumeInfo struct {
	Name       string
	Filename   string
	Format     string
	Fstype     string
	DeviceName string
	ScsiId     int
	Containers []int
	MontPoints []string
}

type PersistNetworkInfo struct {
	Id         string
	Index      int
	PciAddr    int
	DeviceName string
	IpAddr     string
}

type PersistInfo struct {
	PersistVersion int
	Id             string
	DriverInfo     map[string]interface{}
	VmSpec         *hyperstartapi.Pod
	HwStat         *VmHwStatus
	VolumeList     []*PersistVolumeInfo
	NetworkList    []*PersistNetworkInfo
	PortList       []*api.PortDescription
	ContainerList  []*PersistContainerInfo
}

func (ctx *VmContext) dump() (*PersistInfo, error) {
	dr, err := ctx.DCtx.Dump()
	if err != nil {
		return nil, err
	}

	nc := ctx.networks
	info := &PersistInfo{
		PersistVersion: CURRENT_PERSIST_VERSION,
		Id:             ctx.Id,
		DriverInfo:     dr,
		VmSpec:         ctx.networks.sandboxInfo(),
		HwStat:         ctx.dumpHwInfo(),
		VolumeList:     make([]*PersistVolumeInfo, len(ctx.volumes)),
		NetworkList:    make([]*PersistNetworkInfo, len(nc.eth)+len(nc.lo)),
		PortList:       make([]*api.PortDescription, len(nc.ports)),
		ContainerList:  make([]*PersistContainerInfo, len(ctx.containers)),
	}

	vid := 0
	for _, vol := range ctx.volumes {
		info.VolumeList[vid] = vol.dump()
		vid++
	}

	for i, p := range nc.ports {
		info.PortList[i] = &api.PortDescription{
			HostPort:      p.HostPort,
			ContainerPort: p.ContainerPort,
			Protocol:      p.Protocol,
		}
	}
	nid := 0
	for _, nic := range nc.lo {
		info.NetworkList[nid] = &PersistNetworkInfo{
			Id:         nic.Id,
			Index:      nic.Index,
			PciAddr:    nic.PCIAddr,
			DeviceName: nic.DeviceName,
			IpAddr:     nic.IpAddr,
		}
		nid++
	}
	nc.slotLock.RLock()
	for _, nic := range nc.eth {
		info.NetworkList[nid] = &PersistNetworkInfo{
			Id:         nic.Id,
			Index:      nic.Index,
			PciAddr:    nic.PCIAddr,
			DeviceName: nic.DeviceName,
			IpAddr:     nic.IpAddr,
		}
		nid++
	}
	defer nc.slotLock.RUnlock()

	cid := 0
	for _, c := range ctx.containers {
		info.ContainerList[cid] = c.dump()
		cid++
	}

	return info, nil
}

func (ctx *VmContext) dumpHwInfo() *VmHwStatus {
	return &VmHwStatus{
		PciAddr:  ctx.pciAddr,
		ScsiId:   ctx.scsiId,
		AttachId: ctx.hyperstart.LastStreamSeq(),
		GuestCid: ctx.GuestCid,
	}
}

func (ctx *VmContext) loadHwStatus(pinfo *PersistInfo) error {
	ctx.pciAddr = pinfo.HwStat.PciAddr
	ctx.scsiId = pinfo.HwStat.ScsiId
	ctx.GuestCid = pinfo.HwStat.GuestCid
	if ctx.GuestCid != 0 {
		if !VsockCidManager.MarkCidInuse(ctx.GuestCid) {
			return fmt.Errorf("conflicting vsock guest cid %d: already in use", ctx.GuestCid)
		}
		ctx.Boot.EnableVsock = true
	}
	return nil
}

func (blk *DiskDescriptor) dump() *PersistVolumeInfo {
	return &PersistVolumeInfo{
		Name:       blk.Name,
		Filename:   blk.Filename,
		Format:     blk.Format,
		Fstype:     blk.Fstype,
		DeviceName: blk.DeviceName,
		ScsiId:     blk.ScsiId,
	}
}

func (vol *PersistVolumeInfo) blockInfo() *DiskDescriptor {
	return &DiskDescriptor{
		Name:       vol.Name,
		Filename:   vol.Filename,
		Format:     vol.Format,
		Fstype:     vol.Fstype,
		DeviceName: vol.DeviceName,
		ScsiId:     vol.ScsiId,
	}
}

func (nc *NetworkContext) load(pinfo *PersistInfo) {
	nc.SandboxConfig = &api.SandboxConfig{
		Hostname: pinfo.VmSpec.Hostname,
		Dns:      pinfo.VmSpec.Dns,
	}
	portWhilteList := pinfo.VmSpec.PortmappingWhiteLists
	if portWhilteList != nil {
		nc.Neighbors = &api.NeighborNetworks{
			InternalNetworks: portWhilteList.InternalNetworks,
			ExternalNetworks: portWhilteList.ExternalNetworks,
		}
	}

	for i, p := range pinfo.PortList {
		nc.ports[i] = p
	}
	for _, pi := range pinfo.NetworkList {
		ifc := &InterfaceCreated{
			Id:         pi.Id,
			Index:      pi.Index,
			PCIAddr:    pi.PciAddr,
			DeviceName: pi.DeviceName,
			IpAddr:     pi.IpAddr,
		}
		// if empty, may be old data, generate one for compatibility.
		if ifc.Id == "" {
			ifc.Id = utils.RandStr(8, "alpha")
		}
		// use device name distinguish from lo and eth
		if ifc.DeviceName == DEFAULT_LO_DEVICE_NAME {
			nc.lo[pi.IpAddr] = ifc
		} else {
			nc.eth[pi.Index] = ifc
		}
		nc.idMap[pi.Id] = ifc
	}
}

func (cc *ContainerContext) dump() *PersistContainerInfo {
	return &PersistContainerInfo{
		Description: cc.ContainerDescription,
		Root:        cc.root.dump(),
		Process:     cc.process,
		Fsmap:       cc.fsmap,
		VmVolumes:   cc.vmVolumes,
	}
}

func vmDeserialize(s []byte) (*PersistInfo, error) {
	info := &PersistInfo{}
	err := json.Unmarshal(s, info)
	return info, err
}

func (pinfo *PersistInfo) serialize() ([]byte, error) {
	return json.Marshal(pinfo)
}

func (pinfo *PersistInfo) vmContext(hub chan VmEvent, client chan *types.VmResponse) (*VmContext, error) {
	oldVersion := pinfo.PersistVersion < CURRENT_PERSIST_VERSION

	dc, err := HDriver.LoadContext(pinfo.DriverInfo)
	if err != nil {
		glog.Error("cannot load driver context: ", err.Error())
		return nil, err
	}

	ctx, err := InitContext(pinfo.Id, hub, client, dc, &BootConfig{})
	if err != nil {
		return nil, err
	}

	err = ctx.loadHwStatus(pinfo)
	if err != nil {
		return nil, err
	}

	ctx.networks.load(pinfo)

	oldVersionVolume := false
	if len(pinfo.VolumeList) > 0 && (len(pinfo.VolumeList[0].Containers) > 0 || len(pinfo.VolumeList[0].MontPoints) > 0) {
		oldVersionVolume = true
	}
	imageMap := make(map[string]*DiskDescriptor)
	for _, vol := range pinfo.VolumeList {
		binfo := vol.blockInfo()
		if oldVersionVolume {
			if len(vol.Containers) != len(vol.MontPoints) {
				return nil, fmt.Error("persistent data corrupt, volume info mismatch")
			}
			if len(vol.MontPoints) == 1 && vol.MontPoints[0] == "/" {
				imageMap[vol.Name] = binfo
				continue
			}
		}
		ctx.volumes[binfo.Name] = &DiskContext{
			DiskDescriptor: binfo,
			sandbox:        ctx,
			observers:      make(map[string]*sync.WaitGroup),
			lock:           &sync.RWMutex{},
			// FIXME: is there any trouble if we set it as ready when restoring from persistence
			ready: true,
		}
	}

	for _, pc := range pinfo.ContainerList {
		c := pc.Description
		cc := &ContainerContext{
			ContainerDescription: c,
			fsmap:                pc.Fsmap,
			vmVolumes:            pc.VmVolumes,
			process:              pc.Process,
			sandbox:              ctx,
			logPrefix:            fmt.Sprintf("SB[%s] Con[%s] ", ctx.Id, c.Id),
			root: &DiskContext{
				DiskDescriptor: pc.Root.blockInfo(),
				sandbox:        ctx,
				isRootVol:      true,
				ready:          true,
				observers:      make(map[string]*sync.WaitGroup),
				lock:           &sync.RWMutex{},
			},
		}
		// restore wg for volumes attached to container
		wgDisk := &sync.WaitGroup{}
		for vn := range c.Volumes {
			entry, ok := ctx.volumes[vn]
			if !ok {
				cc.Log(ERROR, "restoring container volume does not exist in volume map, skip")
				continue
			}
			entry.wait(c.Id, wgDisk)
		}
		cc.root.wait(c.Id, wgDisk)

		ctx.containers[c.Id] = cc
	}

	return ctx, nil
}
