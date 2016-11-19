package hypervisor

import (
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/hyperhq/runv/api"
	hyperstartapi "github.com/hyperhq/runv/hyperstart/api/json"
	"github.com/hyperhq/runv/hypervisor/types"
)

type VmHwStatus struct {
	PciAddr  int    //next available pci addr for pci hotplug
	ScsiId   int    //next available scsi id for scsi hotplug
	AttachId uint64 //next available attachId for attached tty
}

const (
	PauseStateUnpaused = iota
	PauseStateBusy
	PauseStatePaused
)

type VmContext struct {
	Id string

	PauseState int
	Boot       *BootConfig

	vmHyperstartAPIVersion uint32

	// Communication Context
	Hub    chan VmEvent
	client chan *types.VmResponse
	vm     chan *hyperstartCmd

	DCtx DriverContext

	HomeDir         string
	HyperSockName   string
	TtySockName     string
	ConsoleSockName string
	ShareDir        string

	pciAddr int //next available pci addr for pci hotplug
	scsiId  int //next available scsi id for scsi hotplug

	//	InterfaceCount int

	ptys *pseudoTtys

	// Specification
	volumes    map[string]*DiskContext
	containers map[string]*ContainerContext
	networks   *NetworkContext

	// internal states
	vmExec map[string]*hyperstartapi.ExecCommand

	// Internal Helper
	handler stateHandler
	current string
	timer   *time.Timer

	lock *sync.Mutex //protect update of context
	wg   *sync.WaitGroup
}

type stateHandler func(ctx *VmContext, event VmEvent)

func NewVmSpec() *hyperstartapi.Pod {
	return &hyperstartapi.Pod{
		ShareDir: ShareDirTag,
	}
}

func InitContext(id string, hub chan VmEvent, client chan *types.VmResponse, dc DriverContext, boot *BootConfig) (*VmContext, error) {
	var (
		vmChannel = make(chan *hyperstartCmd, 128)

		//dir and sockets:
		homeDir         = BaseDir + "/" + id + "/"
		hyperSockName   = homeDir + HyperSockName
		ttySockName     = homeDir + TtySockName
		consoleSockName = homeDir + ConsoleSockName
		shareDir        = homeDir + ShareDirTag
		ctx             *VmContext
	)

	err := os.MkdirAll(shareDir, 0755)
	if err != nil {
		ctx.Log(ERROR, "cannot make dir %s: %v", shareDir, err)
		return nil, err
	}

	if dc == nil {
		dc = HDriver.InitContext(homeDir)
		if dc == nil {
			err := fmt.Errorf("cannot create driver context of %s", homeDir)
			ctx.Log(ERROR, "init failed: %v", err)
			return nil, err
		}
	}

	ctx = &VmContext{
		Id:              id,
		Boot:            boot,
		PauseState:      PauseStateUnpaused,
		pciAddr:         PciAddrFrom,
		scsiId:          0,
		Hub:             hub,
		client:          client,
		DCtx:            dc,
		vm:              vmChannel,
		ptys:            newPts(),
		HomeDir:         homeDir,
		HyperSockName:   hyperSockName,
		TtySockName:     ttySockName,
		ConsoleSockName: consoleSockName,
		ShareDir:        shareDir,
		timer:      nil,
		handler:    stateRunning,
		current:    StateRunning,
		volumes:    make(map[string]*DiskContext),
		containers: make(map[string]*ContainerContext),
		networks:   NewNetworkContext(),
		vmExec:     make(map[string]*hyperstartapi.ExecCommand),
		lock:       &sync.Mutex{},
	}
	ctx.networks.sandbox = ctx

	return ctx, nil
}

func (ctx *VmContext) setTimeout(seconds int) {
	if ctx.timer != nil {
		ctx.unsetTimeout()
	}
	ctx.timer = time.AfterFunc(time.Duration(seconds)*time.Second, func() {
		ctx.Hub <- &VmTimeout{}
	})
}

func (ctx *VmContext) unsetTimeout() {
	if ctx.timer != nil {
		ctx.timer.Stop()
		ctx.timer = nil
	}
}

func (ctx *VmContext) reset() {
	ctx.lock.Lock()

	ctx.ptys.closePendingTtys()

	ctx.pciAddr = PciAddrFrom
	ctx.scsiId = 0
	//do not reset attach id here, let it increase

	ctx.containers = make(map[string]*ContainerContext)
	ctx.volumes = make(map[string]*DiskContext)
	ctx.networks = NewNetworkContext()
	ctx.networks.sandbox = ctx

	ctx.lock.Unlock()
}

func (ctx *VmContext) nextScsiId() int {
	ctx.lock.Lock()
	id := ctx.scsiId
	ctx.scsiId++
	ctx.lock.Unlock()
	return id
}

func (ctx *VmContext) nextPciAddr() int {
	ctx.lock.Lock()
	addr := ctx.pciAddr
	ctx.pciAddr++
	ctx.lock.Unlock()
	return addr
}

func (ctx *VmContext) LookupExecBySession(session uint64) string {
	ctx.lock.Lock()
	defer ctx.lock.Unlock()

	for id, exec := range ctx.vmExec {
		if exec.Process.Stdio == session {
			ctx.Log(DEBUG, "found exec %s whose session is %v", id, session)
			return id
		}
	}

	return ""
}

func (ctx *VmContext) DeleteExec(id string) {
	ctx.lock.Lock()
	defer ctx.lock.Unlock()

	delete(ctx.vmExec, id)
}

func (ctx *VmContext) LookupBySession(session uint64) string {
	ctx.lock.Lock()
	defer ctx.lock.Unlock()

	for id, c := range ctx.containers {
		if c.process.Stdio == session {
			ctx.Log(DEBUG, "found container %s whose session is %v", c.Id, session)
			return id
		}
	}
	ctx.Log(DEBUG, "can not found container whose session is %s", session)
	return ""
}

func (ctx *VmContext) Close() {
	ctx.lock.Lock()
	defer ctx.lock.Unlock()
	ctx.ptys.closePendingTtys()
	ctx.unsetTimeout()
	ctx.networks.close()
	ctx.DCtx.Close()
	close(ctx.vm)
	close(ctx.client)
	os.Remove(ctx.ShareDir)
	ctx.handler = nil
	ctx.current = "None"
}

func (ctx *VmContext) Become(handler stateHandler, desc string) {
	orig := ctx.current
	ctx.lock.Lock()
	ctx.handler = handler
	ctx.current = desc
	ctx.lock.Unlock()
	ctx.Log(DEBUG, "state change from %s to '%s'", orig, desc)
}

// User API
func (ctx *VmContext) SetNetworkEnvironment(net *api.SandboxConfig) {
	ctx.lock.Lock()
	defer ctx.lock.Unlock()

	ctx.networks.SandboxConfig = net
}

func (ctx *VmContext) AddPortmapping(ports []*api.PortDescription) {
	ctx.lock.Lock()
	defer ctx.lock.Unlock()
}

func (ctx *VmContext) AddInterface(inf *api.InterfaceDescription, result chan api.Result) {
	ctx.lock.Lock()
	defer ctx.lock.Unlock()

	ctx.networks.addInterface(inf, result)
}

func (ctx *VmContext) RemoveInterface(id string, result chan api.Result) {
	ctx.lock.Lock()
	defer ctx.lock.Unlock()

	ctx.networks.removeInterface(id, result)
}

func (ctx *VmContext) AddContainer(c *api.ContainerDescription, result chan api.Result) {
	ctx.lock.Lock()
	defer ctx.lock.Unlock()

	if ctx.LogLevel(TRACE) {
		ctx.Log(TRACE, "add container %#v", c)
	}

	if _, ok := ctx.containers[c.Id]; ok {
		estr := fmt.Sprintf("duplicate container %s", c.Name)
		ctx.Log(ERROR, estr)
		result <- NewSpecError(c.Id, estr)
		return
	}
	cc := &ContainerContext{
		ContainerDescription: c,
		fsmap:                []*hyperstartapi.FsmapDescriptor{},
		vmVolumes:            []*hyperstartapi.VolumeDescriptor{},
		sandbox:              ctx,
	}

	wgDisk := &sync.WaitGroup{}
	added := []string{}
	rollback := func() {
		for _, d := range added {
			ctx.volumes[d].unwait(c.Id)
		}
	}

	//TODO: should we validate container before we add them to volumeMap?
	for vn := range c.Volumes {
		entry, ok := ctx.volumes[vn]
		if !ok {
			estr := fmt.Sprintf("volume %s does not exist in volume map", vn)
			cc.Log(ERROR, estr)
			rollback()
			result <- NewSpecError(c.Id, estr)
			return
		}

		entry.wait(c.Id, wgDisk)
		added = append(added, vn)
	}

	//prepare runtime environment
	cc.configProcess()

	cc.root = NewDiskContext(ctx, c.RootVolume)
	cc.root.isRootVol = true
	cc.root.insert(nil)
	cc.root.wait(c.Id, wgDisk)

	ctx.containers[c.Id] = cc

	go cc.add(wgDisk, result)

	return
}

func (ctx *VmContext) RemoveContainer(id string, result chan<- api.Result) {
	ctx.lock.Lock()
	defer ctx.lock.Unlock()

	cc, ok := ctx.containers[id]
	if !ok {
		ctx.Log(WARNING, "container %s not exist", id)
		result <- api.NewResultBase(id, true, "not exist")
		return
	}

	for v := range cc.Volumes {
		if vol, ok := ctx.volumes[v]; ok {
			vol.unwait(id)
		}
	}

	cc.root.unwait(id)

	ctx.Log(INFO, "remove container %s", id)
	delete(ctx.containers, id)
	cc.root.remove(result)
}

func (ctx *VmContext) AddVolume(vol *api.VolumeDescription, result chan api.Result) {
	ctx.lock.Lock()
	defer ctx.lock.Unlock()

	if _, ok := ctx.volumes[vol.Name]; ok {
		estr := fmt.Sprintf("duplicate volume %s", vol.Name)
		ctx.Log(WARNING, estr)
		result <- api.NewResultBase(vol.Name, true, estr)
		return
	}

	dc := NewDiskContext(ctx, vol)

	if vol.IsDir() {
		ctx.Log(INFO, "return volume add success for dir %s", vol.Name)
		result <- api.NewResultBase(vol.Name, true, "")
	} else {
		ctx.Log(DEBUG, "insert disk for volume %s", vol.Name)
		dc.insert(result)
	}

	ctx.volumes[vol.Name] = dc
}

func (ctx *VmContext) RemoveVolume(name string, result chan<- api.Result) {
	ctx.lock.Lock()
	defer ctx.lock.Unlock()

	disk, ok := ctx.volumes[name]
	if !ok {
		ctx.Log(WARNING, "volume %s not exist", name)
		result <- api.NewResultBase(name, true, "not exist")
		return
	}

	if disk.containers() > 0 {
		ctx.Log(ERROR, "cannot remove a in use volume %s", name)
		result <- api.NewResultBase(name, false, "in use")
		return
	}

	ctx.Log(INFO, "remove disk %s", name)
	delete(ctx.volumes, name)
	disk.remove(result)
}
