package hypervisor

import (
	"fmt"
	"net"
	"os"
	"strings"

	"github.com/golang/glog"
	"github.com/hyperhq/runv/hypervisor/network"
	"github.com/hyperhq/runv/hypervisor/pod"
)

type deviceMap struct {
	imageMap   map[string]*imageInfo
	volumeMap  map[string]*volume
	networkMap map[int]*InterfaceCreated
}

type BlockDescriptor struct {
	Name       string
	Filename   string
	Format     string
	Fstype     string
	DeviceName string
	ScsiId     int
	Options    map[string]string
}

type imageInfo struct {
	info *BlockDescriptor
	pos  int
}

type volume struct {
	info         *BlockDescriptor
	pos          volumePosition
	readOnly     map[int]bool
	dockerVolume bool
}

type volumePosition map[int]string //containerIdx -> mpoint

type processingList struct {
	adding   *processingMap
	deleting *processingMap
	finished *processingMap
}

type processingMap struct {
	containers  map[int]bool
	volumes     map[string]bool
	blockdevs   map[string]bool
	networks    map[int]bool
	ttys        map[int]bool
	serialPorts map[int]bool
}

func newProcessingMap() *processingMap {
	return &processingMap{
		containers: make(map[int]bool),    //to be create, and get images,
		volumes:    make(map[string]bool), //to be create, and get volume
		blockdevs:  make(map[string]bool), //to be insert to VM, both volume and images
		networks:   make(map[int]bool),
	}
}

func newProcessingList() *processingList {
	return &processingList{
		adding:   newProcessingMap(),
		deleting: newProcessingMap(),
		finished: newProcessingMap(),
	}
}

func newDeviceMap() *deviceMap {
	return &deviceMap{
		imageMap:   make(map[string]*imageInfo),
		volumeMap:  make(map[string]*volume),
		networkMap: make(map[int]*InterfaceCreated),
	}
}

func (pm *processingMap) isEmpty() bool {
	return len(pm.containers) == 0 && len(pm.volumes) == 0 && len(pm.blockdevs) == 0 &&
		len(pm.networks) == 0
}

func (ctx *VmContext) deviceReady() bool {
	ready := ctx.progress.adding.isEmpty() && ctx.progress.deleting.isEmpty()
	if ready && ctx.wait {
		glog.V(1).Info("All resource being released, someone is waiting for us...")
		ctx.wg.Done()
		ctx.wait = false
	}

	return ready
}

func (ctx *VmContext) initContainerInfo(index int, target *VmContainer, spec *pod.UserContainer) {
	vols := []VmVolumeDescriptor{}
	fsmap := []VmFsmapDescriptor{}
	for _, v := range spec.Volumes {
		ctx.devices.volumeMap[v.Volume].pos[index] = v.Path
		ctx.devices.volumeMap[v.Volume].readOnly[index] = v.ReadOnly
	}

	envs := make([]VmEnvironmentVar, len(spec.Envs))
	for j, e := range spec.Envs {
		envs[j] = VmEnvironmentVar{Env: e.Env, Value: e.Value}
	}

	restart := "never"
	if len(spec.RestartPolicy) > 0 {
		restart = spec.RestartPolicy
	}

	p := VmProcess{Terminal: spec.Tty, Stdio: 0, Stderr: 0, Args: spec.Command, Envs: envs, Workdir: spec.Workdir}
	*target = VmContainer{
		Id: "", Rootfs: "rootfs", Fstype: "", Image: "",
		Volumes: vols, Fsmap: fsmap, Process: p, Entrypoint: spec.Entrypoint,
		Sysctl: spec.Sysctl, RestartPolicy: restart,
	}
}

func (ctx *VmContext) setContainerInfo(index int, container *VmContainer, info *ContainerInfo) {

	container.Id = info.Id
	container.Rootfs = info.Rootfs

	container.Process.Args = info.Cmd
	container.Process.Envs = make([]VmEnvironmentVar, len(info.Envs))
	i := 0
	for e, v := range info.Envs {
		container.Process.Envs[i].Env = e
		container.Process.Envs[i].Value = v
		i++
	}

	if container.Process.Workdir == "" {
		if info.Workdir != "" {
			container.Process.Workdir = info.Workdir
		} else {
			container.Process.Workdir = "/"
		}
	}

	container.Initialize = info.Initialize

	if info.Fstype == "dir" {
		container.Image = info.Image
		container.Fstype = ""
	} else {
		container.Fstype = info.Fstype
		ctx.devices.imageMap[info.Image] = &imageInfo{
			info: &BlockDescriptor{
				Name: info.Image, Filename: info.Image, Format: "raw", Fstype: info.Fstype, DeviceName: ""},
			pos: index,
		}
		ctx.progress.adding.blockdevs[info.Image] = true
	}
}

func (ctx *VmContext) initVolumeMap(spec *pod.UserPod) {
	//classify volumes, and generate device info and progress info
	for _, vol := range spec.Volumes {
		v := &volume{
			pos:      make(map[int]string),
			readOnly: make(map[int]bool),
		}

		if vol.Source == "" || vol.Driver == "" {
			v.info = &BlockDescriptor{
				Name:       vol.Name,
				Filename:   "",
				Format:     "",
				Fstype:     "",
				DeviceName: "",
			}
			ctx.devices.volumeMap[vol.Name] = v
		} else if vol.Driver == "raw" || vol.Driver == "qcow2" || vol.Driver == "vdi" {
			v.info = &BlockDescriptor{
				Name:       vol.Name,
				Filename:   vol.Source,
				Format:     vol.Driver,
				Fstype:     "ext4",
				DeviceName: "",
			}
			ctx.devices.volumeMap[vol.Name] = v
			ctx.progress.adding.blockdevs[vol.Name] = true
		} else if vol.Driver == "vfs" {
			v.info = &BlockDescriptor{
				Name:       vol.Name,
				Filename:   vol.Source,
				Format:     vol.Driver,
				Fstype:     "dir",
				DeviceName: "",
			}
			ctx.devices.volumeMap[vol.Name] = v
		} else if vol.Driver == "rbd" {
			v.info = &BlockDescriptor{
				Name:       vol.Name,
				Filename:   vol.Source,
				Format:     vol.Driver,
				Fstype:     "ext4",
				DeviceName: "",
				Options: map[string]string{
					"user":     vol.Option.User,
					"keyring":  vol.Option.Keyring,
					"monitors": strings.Join(vol.Option.Monitors, ";"),
				},
			}
			ctx.devices.volumeMap[vol.Name] = v
			ctx.progress.adding.blockdevs[vol.Name] = true
		}
	}
}

func (ctx *VmContext) setVolumeInfo(info *VolumeInfo) {
	vol, ok := ctx.devices.volumeMap[info.Name]
	if !ok {
		return
	}

	vol.info.Filename = info.Filepath
	vol.info.Format = info.Format
	vol.dockerVolume = info.DockerVolume

	if info.Fstype != "dir" {
		vol.info.Fstype = info.Fstype
		ctx.progress.adding.blockdevs[info.Name] = true
	} else {
		vol.info.Fstype = ""
		for i, mount := range vol.pos {
			glog.V(1).Infof("insert volume %s to %s on %d", info.Name, mount, i)
			ctx.vmSpec.Containers[i].Fsmap = append(ctx.vmSpec.Containers[i].Fsmap, VmFsmapDescriptor{
				Source:       info.Filepath,
				Path:         mount,
				ReadOnly:     vol.readOnly[i],
				DockerVolume: info.DockerVolume,
			})
		}
	}
}

func (ctx *VmContext) allocateNetworks() {
	for i := range ctx.progress.adding.networks {
		name := fmt.Sprintf("eth%d", i)
		addr := ctx.nextPciAddr()
		if len(ctx.userSpec.Interfaces) > 0 {
			go ctx.ConfigureInterface(i, addr, name, ctx.userSpec.Interfaces[i])
		} else {
			go ctx.CreateInterface(i, addr, name)
		}
	}

	for _, srv := range ctx.userSpec.Services {
		inf := VmNetworkInf{
			Device:    "lo",
			IpAddress: srv.ServiceIP,
			NetMask:   "255.255.255.255",
		}

		ctx.vmSpec.Interfaces = append(ctx.vmSpec.Interfaces, inf)
	}
}

func (ctx *VmContext) addBlockDevices() {
	for blk := range ctx.progress.adding.blockdevs {
		if info, ok := ctx.devices.volumeMap[blk]; ok {
			sid := ctx.nextScsiId()
			info.info.ScsiId = sid
			ctx.DCtx.AddDisk(ctx, "volume", info.info)
		} else if info, ok := ctx.devices.imageMap[blk]; ok {
			sid := ctx.nextScsiId()
			info.info.ScsiId = sid
			ctx.DCtx.AddDisk(ctx, "image", info.info)
		} else {
			continue
		}
	}
}

func (ctx *VmContext) allocateDevices() {
	if len(ctx.progress.adding.networks) == 0 && len(ctx.progress.adding.blockdevs) == 0 {
		ctx.Hub <- &DevSkipEvent{}
		return
	}

	ctx.allocateNetworks()
	ctx.addBlockDevices()
}

func (ctx *VmContext) blockdevInserted(info *BlockdevInsertedEvent) {
	ctx.lock.Lock()
	defer ctx.lock.Unlock()

	if info.SourceType == "image" {
		image := ctx.devices.imageMap[info.Name]
		image.info.DeviceName = info.DeviceName
		image.info.ScsiId = info.ScsiId
		ctx.vmSpec.Containers[image.pos].Image = info.DeviceName
		ctx.vmSpec.Containers[image.pos].Addr = info.ScsiAddr
	} else if info.SourceType == "volume" {
		volume := ctx.devices.volumeMap[info.Name]
		volume.info.DeviceName = info.DeviceName
		volume.info.ScsiId = info.ScsiId
		for c, vol := range volume.pos {
			ctx.vmSpec.Containers[c].Volumes = append(ctx.vmSpec.Containers[c].Volumes,
				VmVolumeDescriptor{
					Device:       info.DeviceName,
					Addr:         info.ScsiAddr,
					Mount:        vol,
					Fstype:       volume.info.Fstype,
					ReadOnly:     volume.readOnly[c],
					DockerVolume: volume.dockerVolume,
				})
		}
	}

	ctx.progress.finished.blockdevs[info.Name] = true
	if _, ok := ctx.progress.adding.blockdevs[info.Name]; ok {
		delete(ctx.progress.adding.blockdevs, info.Name)
	}
}

func (ctx *VmContext) interfaceCreated(info *InterfaceCreated) {
	ctx.lock.Lock()
	defer ctx.lock.Unlock()
	ctx.devices.networkMap[info.Index] = info
}

func (ctx *VmContext) netdevInserted(info *NetDevInsertedEvent) {
	ctx.lock.Lock()
	defer ctx.lock.Unlock()
	ctx.progress.finished.networks[info.Index] = true
	if _, ok := ctx.progress.adding.networks[info.Index]; ok {
		delete(ctx.progress.adding.networks, info.Index)
	}
	if len(ctx.progress.adding.networks) == 0 {
		count := len(ctx.devices.networkMap)
		for i := 0; i < count; i++ {
			inf := VmNetworkInf{
				Device:    ctx.devices.networkMap[i].DeviceName,
				IpAddress: ctx.devices.networkMap[i].IpAddr,
				NetMask:   ctx.devices.networkMap[i].NetMask,
			}
			ctx.vmSpec.Interfaces = append(ctx.vmSpec.Interfaces, inf)
			for _, rl := range ctx.devices.networkMap[i].RouteTable {
				dev := ""
				if rl.ViaThis {
					dev = inf.Device
				}
				ctx.vmSpec.Routes = append(ctx.vmSpec.Routes, VmRoute{
					Dest:    rl.Destination,
					Gateway: rl.Gateway,
					Device:  dev,
				})
			}
		}
	}
}

func (ctx *VmContext) onContainerRemoved(c *ContainerUnmounted) bool {
	ctx.lock.Lock()
	defer ctx.lock.Unlock()

	if _, ok := ctx.progress.deleting.containers[c.Index]; ok {
		glog.V(1).Infof("container %d umounted", c.Index)
		delete(ctx.progress.deleting.containers, c.Index)
	}
	if ctx.vmSpec.Containers[c.Index].Fstype != "" {
		for name, image := range ctx.devices.imageMap {
			if image.pos == c.Index {
				glog.V(1).Info("need remove image dm file", image.info.Filename)
				ctx.progress.deleting.blockdevs[name] = true
				go UmountDMDevice(image.info.Filename, name, ctx.Hub)
			}
		}
	}

	return c.Success
}

func (ctx *VmContext) onInterfaceRemoved(nic *InterfaceReleased) bool {
	if _, ok := ctx.progress.deleting.networks[nic.Index]; ok {
		glog.V(1).Infof("interface %d released", nic.Index)
		delete(ctx.progress.deleting.networks, nic.Index)
	}

	return nic.Success
}

func (ctx *VmContext) onVolumeRemoved(v *VolumeUnmounted) bool {
	if _, ok := ctx.progress.deleting.volumes[v.Name]; ok {
		glog.V(1).Infof("volume %s umounted", v.Name)
		delete(ctx.progress.deleting.volumes, v.Name)
	}
	vol := ctx.devices.volumeMap[v.Name]
	if vol.info.Fstype != "" {
		glog.V(1).Info("need remove dm file ", vol.info.Filename)
		ctx.progress.deleting.blockdevs[vol.info.Name] = true
		go UmountDMDevice(vol.info.Filename, vol.info.Name, ctx.Hub)
	}
	return v.Success
}

func (ctx *VmContext) onBlockReleased(v *BlockdevRemovedEvent) bool {
	if _, ok := ctx.progress.deleting.blockdevs[v.Name]; ok {
		glog.V(1).Infof("blockdev %s deleted", v.Name)
		delete(ctx.progress.deleting.blockdevs, v.Name)
	}
	return v.Success
}

func (ctx *VmContext) releaseVolumeDir() {
	for name, vol := range ctx.devices.volumeMap {
		if vol.info.Fstype == "" {
			glog.V(1).Info("need umount dir ", vol.info.Filename)
			ctx.progress.deleting.volumes[name] = true
			go UmountVolume(ctx.ShareDir, vol.info.Filename, name, ctx.Hub)
		}
	}
}

func (ctx *VmContext) removeDMDevice() {
	for name, container := range ctx.devices.imageMap {
		if container.info.Fstype != "dir" {
			glog.V(1).Info("need remove dm file", container.info.Filename)
			ctx.progress.deleting.blockdevs[name] = true
			go UmountDMDevice(container.info.Filename, name, ctx.Hub)
		}
	}
	for name, vol := range ctx.devices.volumeMap {
		if vol.info.Fstype != "" {
			glog.V(1).Info("need remove dm file ", vol.info.Filename)
			ctx.progress.deleting.blockdevs[name] = true
			go UmountDMDevice(vol.info.Filename, name, ctx.Hub)
		}
	}
}

func (ctx *VmContext) releaseOverlayDir() {
	if !supportOverlay() {
		return
	}
	for idx, container := range ctx.vmSpec.Containers {
		if container.Fstype == "" {
			glog.V(1).Info("need unmount overlay dir ", container.Image)
			ctx.progress.deleting.containers[idx] = true
			go UmountOverlayContainer(ctx.ShareDir, container.Image, idx, ctx.Hub)
		}
	}
}

func (ctx *VmContext) releaseAufsDir() {
	if !supportAufs() {
		return
	}
	for idx, container := range ctx.vmSpec.Containers {
		if container.Fstype == "" {
			glog.V(1).Info("need unmount aufs ", container.Image)
			ctx.progress.deleting.containers[idx] = true
			go UmountAufsContainer(ctx.ShareDir, container.Image, idx, ctx.Hub)
		}
	}
}

func (ctx *VmContext) removeVolumeDrive() {
	for name, vol := range ctx.devices.volumeMap {
		if vol.info.Format == "raw" || vol.info.Format == "qcow2" || vol.info.Format == "vdi" || vol.info.Format == "rbd" {
			glog.V(1).Infof("need detach volume %s (%s) ", name, vol.info.DeviceName)
			ctx.DCtx.RemoveDisk(ctx, vol.info, &VolumeUnmounted{Name: name, Success: true})
			ctx.progress.deleting.volumes[name] = true
		}
	}
}

func (ctx *VmContext) removeImageDrive() {
	for _, image := range ctx.devices.imageMap {
		if image.info.Fstype != "dir" {
			glog.V(1).Infof("need eject no.%d image block device: %s", image.pos, image.info.DeviceName)
			ctx.progress.deleting.containers[image.pos] = true
			ctx.DCtx.RemoveDisk(ctx, image.info, &ContainerUnmounted{Index: image.pos, Success: true})
		}
	}
}

func (ctx *VmContext) releaseNetwork() {
	var maps []pod.UserContainerPort

	for _, c := range ctx.userSpec.Containers {
		for _, m := range c.Ports {
			maps = append(maps, m)
		}
	}

	for idx, nic := range ctx.devices.networkMap {
		glog.V(1).Infof("remove network card %d: %s", idx, nic.IpAddr)
		ctx.progress.deleting.networks[idx] = true
		go ctx.ReleaseInterface(idx, nic.IpAddr, nic.Fd, maps)
		maps = nil
	}
}

func (ctx *VmContext) removeInterface() {
	var maps []pod.UserContainerPort

	for _, c := range ctx.userSpec.Containers {
		for _, m := range c.Ports {
			maps = append(maps, m)
		}
	}

	for idx, nic := range ctx.devices.networkMap {
		glog.V(1).Infof("remove network card %d: %s", idx, nic.IpAddr)
		ctx.progress.deleting.networks[idx] = true
		ctx.DCtx.RemoveNic(ctx, nic, &NetDevRemovedEvent{Index: idx})
		maps = nil
	}
}

func (ctx *VmContext) allocateInterface(index int, pciAddr int, name string) (*InterfaceCreated, error) {
	var err error
	var inf *network.Settings
	var maps []pod.UserContainerPort

	if index == 0 {
		for _, c := range ctx.userSpec.Containers {
			for _, m := range c.Ports {
				maps = append(maps, m)
			}
		}
	}

	if HDriver.BuildinNetwork() {
		inf, err = ctx.DCtx.AllocateNetwork(ctx.Id, "", maps)
	} else {
		inf, err = network.Allocate(ctx.Id, "", false, maps)
	}

	if err != nil {
		glog.Error("interface creating failed: ", err.Error())

		return &InterfaceCreated{Index: index, PCIAddr: pciAddr, DeviceName: name}, err
	}

	return interfaceGot(index, pciAddr, name, inf)
}

func (ctx *VmContext) ConfigureInterface(index int, pciAddr int, name string, config pod.UserInterface) {
	var err error
	var inf *network.Settings
	var maps []pod.UserContainerPort

	if index == 0 {
		for _, c := range ctx.userSpec.Containers {
			for _, m := range c.Ports {
				maps = append(maps, m)
			}
		}
	}

	if HDriver.BuildinNetwork() {
		/* VBox doesn't support join to bridge */
		inf, err = ctx.DCtx.ConfigureNetwork(ctx.Id, "", maps, config)
	} else {
		inf, err = network.Configure(ctx.Id, "", false, maps, config)
	}

	if err != nil {
		glog.Error("interface creating failed: ", err.Error())
		session := &InterfaceCreated{Index: index, PCIAddr: pciAddr, DeviceName: name}
		ctx.Hub <- &DeviceFailed{Session: session}
		return
	}

	session, err := interfaceGot(index, pciAddr, name, inf)
	if err != nil {
		ctx.Hub <- &DeviceFailed{Session: session}
		return
	}

	ctx.Hub <- session
}

func (ctx *VmContext) CreateInterface(index int, pciAddr int, name string) {
	session, err := ctx.allocateInterface(index, pciAddr, name)

	if err != nil {
		ctx.Hub <- &DeviceFailed{Session: session}
		return
	}

	ctx.Hub <- session
}

func (ctx *VmContext) ReleaseInterface(index int, ipAddr string, file *os.File,
	maps []pod.UserContainerPort) {
	var err error
	success := true

	if HDriver.BuildinNetwork() {
		err = ctx.DCtx.ReleaseNetwork(ctx.Id, ipAddr, maps, file)
	} else {
		err = network.Release(ctx.Id, ipAddr, maps, file)
	}

	if err != nil {
		glog.Warning("Unable to release network interface, address: ", ipAddr, err)
		success = false
	}
	ctx.Hub <- &InterfaceReleased{Index: index, Success: success}
}

func interfaceGot(index int, pciAddr int, name string, inf *network.Settings) (*InterfaceCreated, error) {
	ip, nw, err := net.ParseCIDR(fmt.Sprintf("%s/%d", inf.IPAddress, inf.IPPrefixLen))
	if err != nil {
		glog.Error("can not parse cidr")
		return &InterfaceCreated{Index: index, PCIAddr: pciAddr, DeviceName: name}, err
	}
	var tmp []byte = nw.Mask
	var mask net.IP = tmp

	rt := []*RouteRule{}
	/* Route rule is generated automaticly on first interface,
	 * or generated on the gateway configured interface. */
	if (index == 0 && inf.Automatic) || (!inf.Automatic && inf.Gateway != "") {
		rt = append(rt, &RouteRule{
			Destination: "0.0.0.0/0",
			Gateway:     inf.Gateway, ViaThis: true,
		})
	}

	return &InterfaceCreated{
		Index:      index,
		PCIAddr:    pciAddr,
		Bridge:     inf.Bridge,
		HostDevice: inf.Device,
		DeviceName: name,
		Fd:         inf.File,
		MacAddr:    inf.Mac,
		IpAddr:     ip.String(),
		NetMask:    mask.String(),
		RouteTable: rt,
	}, nil
}
