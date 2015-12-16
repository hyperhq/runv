package libvirt

import (
	"encoding/xml"
	"fmt"

	libvirtgo "github.com/alexzorin/libvirt-go"
	"github.com/golang/glog"
	"github.com/hyperhq/runv/hypervisor"
	"github.com/hyperhq/runv/lib/utils"
)

var LibvirtdAddress = "qemu:///system"

type LibvirtDriver struct {
	conn    libvirtgo.VirConnection
	domains map[string]*hypervisor.VmContext
}

type LibvirtContext struct {
	driver *LibvirtDriver
	domain *libvirtgo.VirDomain
}

func InitDriver() *LibvirtDriver {
	conn, err := libvirtgo.NewVirConnection(LibvirtdAddress)
	if err != nil {
		glog.Error("fail to connect to libvirtd ", LibvirtdAddress, err)
		return nil
	}

	return &LibvirtDriver{
		conn:    conn,
		domains: make(map[string]*hypervisor.VmContext),
	}
}

func (ld *LibvirtDriver) InitContext(homeDir string) hypervisor.DriverContext {
	return &LibvirtContext{
		driver: ld,
	}
}

func (ld *LibvirtDriver) LoadContext(persisted map[string]interface{}) (hypervisor.DriverContext, error) {
	if t, ok := persisted["hypervisor"]; !ok || t != "libvirt" {
		return nil, fmt.Errorf("wrong driver type %v in persist info, expect libvirt", t)
	}

	name, ok := persisted["name"]
	if !ok {
		return nil, fmt.Errorf("there is no libvirt domain name")
	}

	domain, err := ld.conn.LookupDomainByName(name.(string))
	if err != nil {
		return nil, fmt.Errorf("cannot find domain whose name is %v", name)
	}

	return &LibvirtContext{
		driver: ld,
		domain: &domain,
	}, nil
}

func (ld *LibvirtDriver) SupportLazyMode() bool {
	return false
}

type memory struct {
	Unit    string `xml:"unit,attr"`
	Content int    `xml:",chardata"`
}

type vcpu struct {
	Placement string `xml:"placement,attr"`
	Content   int    `xml:",chardata"`
}

type cpu struct {
	Mode string `xml:"mode,attr"`
}

type ostype struct {
	Arch    string `xml:"arch,attr"`
	Machine string `xml:"machine,attr"`
	Content string `xml:",chardata"`
}

type osloader struct {
	Type     string `xml:"type,attr"`
	ReadOnly string `xml:"readonly,attr"`
	Content  string `xml:",chardata"`
}

type domainos struct {
	Supported string    `xml:"supported,attr"`
	Type      ostype    `xml:"type"`
	Kernel    string    `xml:"kernel,omitempty"`
	Initrd    string    `xml:"initrd,omitempty"`
	Cmdline   string    `xml:"cmdline,omitempty"`
	Loader    *osloader `xml:"loader,omitempty"`
	Nvram     string    `xml:"nvram,omitempty"`
}

type feature struct {
	Acpi string `xml:"acpi"`
}

type address struct {
	Type       string `xml:"type,attr"`
	Domain     string `xml:"domain,attr,omitempty"`
	Controller string `xml:"controller,attr,omitempty"`
	Bus        string `xml:"bus,attr"`
	Slot       string `xml:"slot,attr,omitempty"`
	Function   string `xml:"function,attr,omitempty"`
	Target     string `xml:"target,attr,omitempty"`
	Unit       int    `xml:"uint,attr,omitempty"`
}

type controller struct {
	Type    string   `xml:"type,attr"`
	Index   string   `xml:"index,attr"`
	Model   string   `xml:"model,attr,omitempty"`
	Address *address `xml:"address"`
}

type fsdriver struct {
	Type string `xml:"type,attr"`
}

type fspath struct {
	Dir string `xml:"dir,attr"`
}

type filesystem struct {
	Type       string   `xml:"type,attr"`
	Accessmode string   `xml:"accessmode,attr"`
	Driver     fsdriver `xml:"driver"`
	Source     fspath   `xml:"source"`
	Target     fspath   `xml:"target"`
	Address    *address `xml:"address"`
}

type channsrc struct {
	Mode string `xml:"mode,attr"`
	Path string `xml:"path,attr"`
}

type channtgt struct {
	Type string `xml:"type,attr"`
	Name string `xml:"name,attr"`
}

type channel struct {
	Type   string   `xml:"type,attr"`
	Source channsrc `xml:"source"`
	Target channtgt `xml:"target"`
}

type constgt struct {
	Type string `xml:"type,attr"`
	Port string `xml:"port,attr"`
}

type console struct {
	Type   string   `xml:"type,attr"`
	Source channsrc `xml:"source"`
	Target constgt  `xml:"target"`
}

type device struct {
	Emulator    string       `xml:"emulator"`
	Controllers []controller `xml:"controller"`
	Filesystems []filesystem `xml:"filesystem"`
	Channels    []channel    `xml:"channel"`
	Console     console      `xml:"console"`
}

type domain struct {
	XMLName xml.Name `xml:"domain"`
	Type    string   `xml:"type,attr"`
	Name    string   `xml:"name"`
	Memory  memory   `xml:"memory"`
	VCpu    vcpu     `xml:"vcpu"`
	OS      domainos `xml:"os"`
	Feature feature  `xml:"feature"`
	CPU     cpu      `xml:"cpu"`
	Devices device   `xml:"devices"`
}

func (lc *LibvirtContext) domainXml(ctx *hypervisor.VmContext) (string, error) {
	if ctx.Boot == nil {
		ctx.Boot = &hypervisor.BootConfig{
			CPU:    1,
			Memory: 128,
			Kernel: hypervisor.DefaultKernel,
			Initrd: hypervisor.DefaultInitrd,
		}
	}
	boot := ctx.Boot

	dom := &domain{
		Type: "kvm",
		Name: ctx.Id,
	}

	dom.Memory.Unit = "KiB"
	dom.Memory.Content = ctx.Boot.Memory

	dom.VCpu.Placement = "static"
	dom.VCpu.Content = ctx.Boot.CPU

	dom.OS.Supported = "yes"
	dom.OS.Type.Arch = "x86_64"
	dom.OS.Type.Machine = "pc-i440fx-2.0"
	dom.OS.Type.Content = "hvm"

	dom.CPU.Mode = "host-passthrough"

	dom.Devices.Emulator = "/usr/bin/qemu-kvm"

	pcicontroller := controller{
		Type:  "pci",
		Index: "0",
		Model: "pci-root",
	}
	dom.Devices.Controllers = append(dom.Devices.Controllers, pcicontroller)

	serialcontroller := controller{
		Type:  "virtio-serial",
		Index: "0",
		Address: &address{
			Type:     "pci",
			Domain:   "0x0000",
			Bus:      "0x00",
			Slot:     "0x02",
			Function: "0x00",
		},
	}
	dom.Devices.Controllers = append(dom.Devices.Controllers, serialcontroller)

	scsicontroller := controller{
		Type:  "scsi",
		Index: "0",
		Model: "virtio-scsi",
		Address: &address{
			Type:     "pci",
			Domain:   "0x0000",
			Bus:      "0x00",
			Slot:     "0x03",
			Function: "0x00",
		},
	}
	dom.Devices.Controllers = append(dom.Devices.Controllers, scsicontroller)

	sharedfs := filesystem{
		Type:       "mount",
		Accessmode: "squash",
		Driver: fsdriver{
			Type: "path",
		},
		Source: fspath{
			Dir: ctx.ShareDir,
		},
		Target: fspath{
			Dir: hypervisor.ShareDirTag,
		},
		Address: &address{
			Type:     "pci",
			Domain:   "0x0000",
			Bus:      "0x00",
			Slot:     "0x04",
			Function: "0x00",
		},
	}
	dom.Devices.Filesystems = append(dom.Devices.Filesystems, sharedfs)

	hyperchannel := channel{
		Type: "unix",
		Source: channsrc{
			Mode: "bind",
			Path: ctx.HyperSockName,
		},
		Target: channtgt{
			Type: "virtio",
			Name: "sh.hyper.channel.0",
		},
	}
	dom.Devices.Channels = append(dom.Devices.Channels, hyperchannel)

	ttychannel := channel{
		Type: "unix",
		Source: channsrc{
			Mode: "bind",
			Path: ctx.TtySockName,
		},
		Target: channtgt{
			Type: "virtio",
			Name: "sh.hyper.channel.1",
		},
	}
	dom.Devices.Channels = append(dom.Devices.Channels, ttychannel)

	dom.Devices.Console = console{
		Type: "unix",
		Source: channsrc{
			Mode: "bind",
			Path: ctx.ConsoleSockName,
		},
		Target: constgt{
			Type: "serial",
			Port: "0",
		},
	}

	if boot.Bios != "" && boot.Cbfs != "" {
		dom.OS.Loader = &osloader{
			ReadOnly: "yes",
			Type:     "pflash",
			Content:  boot.Bios,
		}
		dom.OS.Nvram = boot.Cbfs
	} else {
		dom.OS.Kernel = boot.Kernel
		dom.OS.Initrd = boot.Initrd
		dom.OS.Cmdline = "console=ttyS0 panic=1 no_timer_check"
	}

	data, err := xml.Marshal(dom)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (lc *LibvirtContext) Launch(ctx *hypervisor.VmContext) {
	domainXml, err := lc.domainXml(ctx)
	if err != nil {
		glog.Error("Fail to get domain xml configuration:", err)
		ctx.Hub <- &hypervisor.VmStartFailEvent{Message: err.Error()}
		return
	}
	domain, err := lc.driver.conn.DomainCreateXML(domainXml, libvirtgo.VIR_DOMAIN_START_AUTODESTROY)
	if err != nil {
		glog.Error("Fail to launch domain ", err)
		ctx.Hub <- &hypervisor.VmStartFailEvent{Message: err.Error()}
		return
	}
	lc.domain = &domain
	name, err := lc.domain.GetName()
	if err != nil {
		glog.Error("Fail to get domain name ", err)
		ctx.Hub <- &hypervisor.VmStartFailEvent{Message: err.Error()}
		return
	}
	lc.driver.domains[name] = ctx
}

func (lc *LibvirtContext) Associate(ctx *hypervisor.VmContext) {
	name, err := lc.domain.GetName()
	if err != nil {
		glog.Error("Fail to get domain name ", err)
		return
	}
	lc.driver.domains[name] = ctx
}

func (lc *LibvirtContext) Dump() (map[string]interface{}, error) {
	if lc.domain == nil {
		return nil, fmt.Errorf("Dom is invalid")
	}

	name, err := lc.domain.GetName()
	if err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"hypervisor": "libvirt",
		"name":       name,
	}, nil
}

func (lc *LibvirtContext) Shutdown(ctx *hypervisor.VmContext) {
	go func() {
		name, err := lc.domain.GetName()
		if err != nil {
			return
		}
		lc.domain.ShutdownFlags(libvirtgo.VIR_DOMAIN_SHUTDOWN_DEFAULT)
		delete(lc.driver.domains, name)
		ctx.Hub <- &hypervisor.VmExit{}
	}()
}

func (lc *LibvirtContext) Kill(ctx *hypervisor.VmContext) {
	go func() {
		name, err := lc.domain.GetName()
		if err != nil {
			return
		}
		lc.domain.DestroyFlags(libvirtgo.VIR_DOMAIN_DESTROY_DEFAULT)
		delete(lc.driver.domains, name)
		ctx.Hub <- &hypervisor.VmKilledEvent{Success: true}
	}()
}

func (lc *LibvirtContext) Close() {
	lc.domain = nil
}

type diskdriver struct {
	Type string `xml:type,attr`
	Name string `xml:"name,attr,omitempty"`
}

type disksrc struct {
	File string `xml:"file,attr"`
}

type disktgt struct {
	Dev string `xml:"dev,attr,omitempty"`
	Bus string `xml:"bus,attr"`
}

type disk struct {
	XMLName xml.Name   `xml:"disk"`
	Type    string     `xml:"type,attr"`
	Device  string     `xml:"device,attr"`
	Driver  diskdriver `xml:"driver"`
	Source  disksrc    `xml:"source"`
	Target  disktgt    `xml:"target"`
	Address *address   `xml:"address"`
}

func diskXml(filename, format string, id int) (string, error) {
	d := disk{
		Type:   "file",
		Device: "disk",
		Driver: diskdriver{
			Type: format,
		},
		Source: disksrc{
			File: filename,
		},
		Target: disktgt{
			Bus: "scsi",
		},
		Address: &address{
			Type:       "drive",
			Controller: "0",
			Bus:        "0x00",
			Target:     "0",
			Unit:       id,
		},
	}

	data, err := xml.Marshal(d)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func scsiId2Name(id int) string {
	return "sd" + utils.DiskId2Name(id)
}

func (lc *LibvirtContext) AddDisk(ctx *hypervisor.VmContext, name, sourceType, filename, format string, id int) {
	diskXml, err := diskXml(filename, format, id)
	if err != nil {
		return
	}
	lc.domain.AttachDeviceFlags(diskXml, libvirtgo.VIR_DOMAIN_DEVICE_MODIFY_LIVE)
	devName := scsiId2Name(id)
	ctx.Hub <- &hypervisor.BlockdevInsertedEvent{
		Name:       name,
		SourceType: sourceType,
		DeviceName: devName,
		ScsiId:     id,
	}
}

func (lc *LibvirtContext) RemoveDisk(ctx *hypervisor.VmContext, filename, format string, id int, callback hypervisor.VmEvent) {
	diskXml, err := diskXml(filename, format, id)
	if err != nil {
		return
	}
	lc.domain.DetachDeviceFlags(diskXml, libvirtgo.VIR_DOMAIN_DEVICE_MODIFY_LIVE)
	ctx.Hub <- callback
}

type nicmac struct {
	Address string `xml:"address,attr"`
}

type nicsrc struct {
	Bridge string `xml:"bridge,attr"`
}

type nictgt struct {
	Device string `xml:"dev,attr"`
}

type nicmodel fsdriver

type nic struct {
	XMLName xml.Name `xml:"interface"`
	Type    string   `xml:"type,attr"`
	Mac     nicmac   `xml:"mac"`
	Source  nicsrc   `xml:"source"`
	Target  nictgt   `xml:"target"`
	Model   nicmodel `xml:"model"`
	Address *address `xml:"address"`
}

func nicXml(bridge, device, mac string, addr int) (string, error) {
	slot := fmt.Sprintf("0x%x", addr)
	n := nic{
		Type: "bridge",
		Mac: nicmac{
			Address: mac,
		},
		Source: nicsrc{
			Bridge: bridge,
		},
		Target: nictgt{
			Device: device,
		},
		Model: nicmodel{
			Type: "virtio",
		},
		Address: &address{
			Type:     "pci",
			Domain:   "0x0000",
			Bus:      "0x00",
			Slot:     slot,
			Function: "0x0",
		},
	}

	data, err := xml.Marshal(n)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (lc *LibvirtContext) AddNic(ctx *hypervisor.VmContext, host *hypervisor.HostNicInfo, guest *hypervisor.GuestNicInfo) {
	nicXml, err := nicXml(host.Bridge, host.Device, host.Mac, guest.Busaddr)
	if err != nil {
		return
	}

	lc.domain.AttachDeviceFlags(nicXml, libvirtgo.VIR_DOMAIN_DEVICE_MODIFY_LIVE)
	ctx.Hub <- &hypervisor.NetDevInsertedEvent{
		Index:      guest.Index,
		DeviceName: guest.Device,
		Address:    guest.Busaddr,
	}
}

func (lc *LibvirtContext) RemoveNic(ctx *hypervisor.VmContext, device, mac string, callback hypervisor.VmEvent) {
	/* FIXME: pass correct bridge and picaddr*/
	nicXml, err := nicXml("null", device, mac, 1)
	if err != nil {
		return
	}
	lc.domain.DetachDeviceFlags(nicXml, libvirtgo.VIR_DOMAIN_DEVICE_MODIFY_LIVE)
	ctx.Hub <- callback
}
