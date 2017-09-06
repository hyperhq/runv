package main

import (
	"encoding/gob"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"

	"github.com/golang/glog"
	"github.com/hyperhq/runv/api"
	_ "github.com/hyperhq/runv/cli/nsenter"
	"github.com/hyperhq/runv/hypervisor"
	"github.com/kardianos/osext"
	"github.com/urfave/cli"
	"github.com/vishvananda/netlink"
)

type NetlinkUpdateType string

const (
	UpdateTypeLink  NetlinkUpdateType = "link"
	UpdateTypeAddr  NetlinkUpdateType = "addr"
	UpdateTypeRoute NetlinkUpdateType = "route"
)

// NetlinkUpdate tracks the change of network namespace.
type NetlinkUpdate struct {
	// AddrUpdate is used to pass information back from AddrSubscribe()
	Addr netlink.AddrUpdate
	// RouteUpdate is used to pass information back from RouteSubscribe()
	Route netlink.RouteUpdate
	// Veth is used to pass information back from LinkSubscribe().
	// We only support veth link at present.
	Veth *netlink.Veth

	// UpdateType indicates which part of the netlink information has been changed.
	UpdateType NetlinkUpdateType
}

type InterfaceInfo struct {
	Index     int
	PeerIndex int
	Ip        string
}

type nsListener struct {
	enc *gob.Encoder
	dec *gob.Decoder
	cmd *exec.Cmd
}

func GetBridgeFromIndex(idx int) (string, string, error) {
	var attr, bridge *netlink.LinkAttrs
	var options string

	links, err := netlink.LinkList()
	if err != nil {
		glog.Error(err)
		return "", "", err
	}

	for _, link := range links {
		if link.Type() != "veth" {
			continue
		}

		if link.Attrs().Index == idx {
			attr = link.Attrs()
			break
		}
	}

	if attr == nil {
		return "", "", fmt.Errorf("cann't find nic whose ifindex is %d", idx)
	}

	for _, link := range links {
		if link.Type() != "bridge" && link.Type() != "openvswitch" {
			continue
		}

		if link.Attrs().Index == attr.MasterIndex {
			bridge = link.Attrs()
			break
		}
	}

	if bridge == nil {
		return "", "", fmt.Errorf("cann't find bridge contains nic whose ifindex is %d", idx)
	}

	if bridge.Name == "ovs-system" {
		veth, err := netlink.LinkByIndex(idx)
		if err != nil {
			return "", "", err
		}

		out, err := exec.Command("ovs-vsctl", "port-to-br", veth.Attrs().Name).CombinedOutput()
		if err != nil {
			return "", "", err
		}
		bridge.Name = strings.TrimSpace(string(out))

		out, err = exec.Command("ovs-vsctl", "get", "port", veth.Attrs().Name, "tag").CombinedOutput()
		if err != nil {
			return "", "", err
		}
		options = "tag=" + strings.TrimSpace(string(out))
	}

	glog.V(3).Infof("find bridge %s", bridge.Name)

	return bridge.Name, options, nil
}

func initSandboxNetwork(vm *hypervisor.Vm, enc *gob.Encoder, dec *gob.Decoder) error {
	/* send collect netns request to nsListener */
	if err := enc.Encode("init"); err != nil {
		glog.Errorf("listener.dec.Decode init error: %v", err)
		return err
	}

	infos := []InterfaceInfo{}
	/* read nic information of ns from pipe */
	err := dec.Decode(&infos)
	if err != nil {
		glog.Error("listener.dec.Decode infos error: %v", err)
		return err
	}

	routes := []netlink.Route{}
	err = dec.Decode(&routes)
	if err != nil {
		glog.Error("listener.dec.Decode route error: %v", err)
		return err
	}

	var gw_route *netlink.Route
	for idx, route := range routes {
		if route.Dst == nil {
			gw_route = &routes[idx]
		}
	}

	glog.V(3).Infof("interface configuration for sandbox ns is %#v", infos)
	for _, info := range infos {
		bridge, options, err := GetBridgeFromIndex(info.PeerIndex)
		if err != nil {
			glog.Error(err)
			continue
		}

		nicId := strconv.Itoa(info.Index)

		conf := &api.InterfaceDescription{
			Id:      nicId, //ip as an id
			Lo:      false,
			Bridge:  bridge,
			Ip:      info.Ip,
			Options: options,
		}

		if gw_route != nil && gw_route.LinkIndex == info.Index {
			conf.Gw = gw_route.Gw.String()
		}

		// TODO(hukeping): the name here is always eth1, 2, 3, 4, 5, etc.,
		// which would not be the proper way to name device name, instead it
		// should be the same as what we specified in the network namespace.
		//err = hp.vm.AddNic(info.Index, fmt.Sprintf("eth%d", idx), conf)
		err = vm.AddNic(conf)
		if err != nil {
			glog.Error(err)
			return err
		}
	}

	err = vm.AddRoute()
	if err != nil {
		glog.Error(err)
		return err
	}

	// TODO: does nsListener need to be long living?
	//go nsListenerStrap(vm, enc *gob.Encoder, dec *gob.Decoder)

	return nil
}

func nsListenerStrap(vm *hypervisor.Vm, enc *gob.Encoder, dec *gob.Decoder) {
	// Keep watching container network setting
	// and then update vm/hyperstart
	for {
		update := NetlinkUpdate{}
		err := dec.Decode(&update)
		if err != nil {
			if err == io.EOF {
				glog.V(3).Infof("listener.dec.Decode NetlinkUpdate: %v", err)
				break
			}
			glog.Error("listener.dec.Decode NetlinkUpdate error: %v", err)
			continue
		}

		glog.V(3).Infof("network namespace information of %s has been changed", update.UpdateType)
		switch update.UpdateType {
		case UpdateTypeLink:
			link := update.Veth
			if link.Attrs().ParentIndex == 0 {
				glog.V(3).Infof("The deleted link: %s", link)
				err = vm.DeleteNic(strconv.Itoa(link.Attrs().Index))
				if err != nil {
					glog.Error(err)
					continue
				}

			} else {
				glog.V(3).Infof("The changed link: %s", link)
			}

		case UpdateTypeAddr:
			glog.V(3).Infof("The changed address: %s", update.Addr)

			link := update.Veth

			// If there is a delete operation upon an link, it will also trigger
			// the address change event which the link will be NIL since it has
			// already been deleted before the address change event be triggered.
			if link == nil {
				glog.V(3).Info("Link for this address has already been deleted.")
				continue
			}

			// This is just a sanity check.
			//
			// The link should be the one which the address on it has been changed.
			if link.Attrs().Index != update.Addr.LinkIndex {
				glog.Errorf("Get the wrong link with ID %d, expect %d", link.Attrs().Index, update.Addr.LinkIndex)
				continue
			}

			bridge, options, err := GetBridgeFromIndex(link.Attrs().ParentIndex)
			if err != nil {
				glog.Error(err)
				continue
			}

			inf := &api.InterfaceDescription{
				Id:      strconv.Itoa(link.Attrs().Index),
				Lo:      false,
				Bridge:  bridge,
				Ip:      update.Addr.LinkAddress.String(),
				Options: options,
			}

			err = vm.AddNic(inf)
			if err != nil {
				glog.Error(err)
				continue
			}

		case UpdateTypeRoute:

		}
	}
}

func newPipe() (parent, child *os.File, err error) {
	fds, err := syscall.Socketpair(syscall.AF_LOCAL, syscall.SOCK_STREAM|syscall.SOCK_CLOEXEC, 0)
	if err != nil {
		return nil, nil, err
	}
	return os.NewFile(uintptr(fds[1]), "parent"), os.NewFile(uintptr(fds[0]), "child"), nil
}

func startNsListener(options runvOptions, vm *hypervisor.Vm) (err error) {
	var parentPipe, childPipe *os.File
	var path string

	path, err = osext.Executable()
	if err != nil {
		glog.Errorf("cannot find self executable path for %s: %v", os.Args[0], err)
		return err
	}

	glog.V(3).Infof("get exec path %s", path)
	parentPipe, childPipe, err = newPipe()
	if err != nil {
		glog.Errorf("create pipe for network-nslisten failed: %v", err)
		return err
	}

	defer func() {
		if err != nil {
			parentPipe.Close()
			childPipe.Close()
		}
	}()

	cmd := exec.Cmd{
		Path:       path,
		Args:       []string{"runv", "network-nslisten"},
		Env:        append(os.Environ(), fmt.Sprintf("_RUNVNETNSPID=%d", options.withContainer.Pid)),
		ExtraFiles: []*os.File{childPipe},
	}
	if err = cmd.Start(); err != nil {
		glog.Errorf("start network-nslisten failed: %v", err)
		return err
	}

	childPipe.Close()

	enc := gob.NewEncoder(parentPipe)
	dec := gob.NewDecoder(parentPipe)

	defer func() {
		if err != nil {
			cmd.Process.Kill()
		}
		cmd.Wait()
	}()

	/* Make sure nsListener create new netns */
	var ready string
	if err = dec.Decode(&ready); err != nil {
		glog.Errorf("Get ready message from network-nslisten failed: %v", err)
		return err
	}

	if ready != "init" {
		err = fmt.Errorf("get incorrect init message from network-nslisten: %s", ready)
		return err
	}

	err = initSandboxNetwork(vm, enc, dec)
	if err != nil {
		return err
	}
	glog.V(1).Infof("nsListener pid is %d", cmd.Process.Pid)
	return nil
}

var nsListenCommand = cli.Command{
	Name:     "network-nslisten",
	Usage:    "[internal command] collection net namespace's network configuration",
	HideHelp: true,
	Before: func(context *cli.Context) error {
		return cmdPrepare(context, false, false)
	},
	Action: func(context *cli.Context) error {
		doListen()
		return nil
	},
}

func doListen() {

	childPipe := os.NewFile(uintptr(3), "child")
	enc := gob.NewEncoder(childPipe)
	dec := gob.NewDecoder(childPipe)

	/* notify `runv create` to execute prestart hooks */
	if err := enc.Encode("init"); err != nil {
		glog.Error(err)
		return
	}

	/* after execute prestart hooks */
	var ready string
	if err := dec.Decode(&ready); err != nil {
		glog.Error(err)
		return
	}

	if ready != "init" {
		glog.Errorf("get incorrect init message: %s", ready)
		return
	}

	// Get network namespace info for the first time and send to the `runv create`
	/* get route info before link down */
	routes, err := netlink.RouteList(nil, netlink.FAMILY_V4)
	if err != nil {
		glog.Error(err)
		return
	}

	/* send interface info to `runv create` */
	infos := collectionInterfaceInfo()
	if err := enc.Encode(infos); err != nil {
		glog.Error(err)
		return
	}

	if err := enc.Encode(routes); err != nil {
		glog.Error(err)
		return
	}

	// This is a call back function.
	// Use to send netlink update informations to `runv create`.
	//netNs2Containerd := func(netlinkUpdate NetlinkUpdate) {
	//	if err := enc.Encode(netlinkUpdate); err != nil {
	//		glog.Info("err Encode(netlinkUpdate) is :", err)
	//	}
	//}
	// todo: Keep collecting network namespace info and sending to the runv
	//setupNetworkNsTrap(netNs2Containerd)
}

func collectionInterfaceInfo() []InterfaceInfo {
	infos := []InterfaceInfo{}

	links, err := netlink.LinkList()
	if err != nil {
		glog.Error(err)
		return infos
	}

	for _, link := range links {
		if link.Type() != "veth" {
			// lo is here too
			continue
		}

		addrs, err := netlink.AddrList(link, netlink.FAMILY_V4)
		if err != nil {
			glog.Error(err)
			return infos
		}

		for _, addr := range addrs {
			info := InterfaceInfo{
				Ip:        addr.IPNet.String(),
				Index:     link.Attrs().Index,
				PeerIndex: link.Attrs().ParentIndex,
			}
			glog.Infof("get interface %v", info)
			infos = append(infos, info)
		}

		// set link down, tap device take over it
		netlink.LinkSetDown(link)
	}
	return infos
}

// This function should be put into the main process or somewhere that can be
// use to init the network namespace trap.
func setupNetworkNsTrap(netNs2Containerd func(NetlinkUpdate)) {
	// Subscribe for links change event
	chLink := make(chan netlink.LinkUpdate)
	doneLink := make(chan struct{})
	defer close(doneLink)
	if err := netlink.LinkSubscribe(chLink, doneLink); err != nil {
		glog.Fatal(err)
	}

	// Subscribe for addresses change event
	chAddr := make(chan netlink.AddrUpdate)
	doneAddr := make(chan struct{})
	defer close(doneAddr)
	if err := netlink.AddrSubscribe(chAddr, doneAddr); err != nil {
		glog.Fatal(err)
	}

	// Subscribe for route change event
	chRoute := make(chan netlink.RouteUpdate)
	doneRoute := make(chan struct{})
	defer close(doneRoute)
	if err := netlink.RouteSubscribe(chRoute, doneRoute); err != nil {
		glog.Fatal(err)
	}

	for {
		select {
		case updateLink := <-chLink:
			handleLink(updateLink, netNs2Containerd)
		case updateAddr := <-chAddr:
			handleAddr(updateAddr, netNs2Containerd)
		case updateRoute := <-chRoute:
			handleRoute(updateRoute, netNs2Containerd)
		}
	}
}

// Link specific
func handleLink(update netlink.LinkUpdate, callback func(NetlinkUpdate)) {
	if update.IfInfomsg.Flags&syscall.IFF_UP == 1 {
		fmt.Printf("[Link device up]\tupdateLink is:%+v, flag is:0x%x\n", update.Link.Attrs(), update.IfInfomsg.Flags)
	} else {
		if update.Link.Attrs().ParentIndex == 0 {
			fmt.Printf("[Link device !up][Deleted]\tupdateLink is:%+v, flag is:0x%x\n", update.Link.Attrs(), update.IfInfomsg.Flags)
		} else {
			fmt.Printf("[Link device !up]\tupdateLink is:%+v, flag is:0x%x\n", update.Link.Attrs(), update.IfInfomsg.Flags)
		}
	}

	netlinkUpdate := NetlinkUpdate{}
	netlinkUpdate.UpdateType = UpdateTypeLink

	// We would like to only handle the veth pair link at present.
	if veth, ok := (update.Link).(*netlink.Veth); ok {
		netlinkUpdate.Veth = veth
		callback(netlinkUpdate)
	}
}

// Address specific
func handleAddr(update netlink.AddrUpdate, callback func(NetlinkUpdate)) {
	if update.NewAddr {
		fmt.Printf("[Add a address]")
	} else {
		fmt.Printf("[Delete a address]")
	}

	if update.LinkAddress.IP.To4() != nil {
		fmt.Printf("[IPv4]\t%+v\n", update)
	} else {
		// We would not like to handle IPv6 at present.
		fmt.Printf("[IPv6]\t%+v\n", update)
		return
	}

	netlinkUpdate := NetlinkUpdate{}
	netlinkUpdate.Addr = update
	netlinkUpdate.UpdateType = UpdateTypeAddr
	links, err := netlink.LinkList()
	if err != nil {
		glog.Error(err)
	}
	for _, link := range links {
		if link.Attrs().Index == update.LinkIndex && link.Type() == "veth" {
			netlinkUpdate.Veth = link.(*netlink.Veth)
			break
		}
	}
	callback(netlinkUpdate)
}

// Route specific
func handleRoute(update netlink.RouteUpdate, callback func(NetlinkUpdate)) {
	// Route type is not a bit mask for a couple of values, but a single
	// unsigned int, that's why we use switch here not the "&" operator.
	switch update.Type {
	case syscall.RTM_NEWROUTE:
		fmt.Printf("[Create a route]\t%+v\n", update)
	case syscall.RTM_DELROUTE:
		fmt.Printf("[Remove a route]\t%+v\n", update)
	case syscall.RTM_GETROUTE:
		fmt.Printf("[Receive info of a route]\t%+v\n", update)
	}

	netlinkUpdate := NetlinkUpdate{}
	netlinkUpdate.Route = update
	netlinkUpdate.UpdateType = UpdateTypeRoute
	callback(netlinkUpdate)
}
