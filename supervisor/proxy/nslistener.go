package proxy

import (
	"encoding/gob"
	"fmt"
	"os"
	"runtime"
	"syscall"

	"github.com/docker/docker/pkg/reexec"
	"github.com/golang/glog"
	"github.com/hyperhq/runv/supervisor"
	"github.com/vishvananda/netlink"
)

func init() {
	reexec.Register("containerd-nslistener", setupNsListener)
}

func setupNsListener() {
	// FOR DEBUG PURPOSE
	// If you need to debug nslistener, uncomment these lines
	// ==============DEBUG BEGIN=====================
	// flag.CommandLine.Parse([]string{"-v", "3", "--log_dir", "/tmp/", "--alsologtostderr"})
	// =============DEBUG END=======================

	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	/* create own netns */
	if err := syscall.Unshare(syscall.CLONE_NEWNET); err != nil {
		glog.Error(err)
		return
	}

	childPipe := os.NewFile(uintptr(3), "child")
	enc := gob.NewEncoder(childPipe)
	dec := gob.NewDecoder(childPipe)
	gob.Register(&netlink.Veth{})

	/* notify containerd to execute prestart hooks */
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

	// Get network namespace info for the first time and send to the containerd
	/* get route info before link down */
	routes, err := netlink.RouteList(nil, netlink.FAMILY_V4)
	if err != nil {
		glog.Error(err)
		return
	}

	/* send interface info to containerd */
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
	// Use to send netlink update informations to containerd.
	netNs2Containerd := func(netlinkUpdate supervisor.NetlinkUpdate) {
		if err := enc.Encode(netlinkUpdate); err != nil {
			glog.Info("err Encode(netlinkUpdate) is :", err)
		}
	}
	// Keep collecting network namespace info and sending to the containerd
	setupNetworkNsTrap(netNs2Containerd)
}

func collectionInterfaceInfo() []supervisor.InterfaceInfo {
	infos := []supervisor.InterfaceInfo{}

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

		mac := link.Attrs().HardwareAddr
		addrs, err := netlink.AddrList(link, netlink.FAMILY_V4)
		if err != nil {
			glog.Error(err)
			return infos
		}

		for _, addr := range addrs {
			info := supervisor.InterfaceInfo{
				Ip:        addr.IPNet.String(),
				Mac:       mac.String(),
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
func setupNetworkNsTrap(netNs2Containerd func(supervisor.NetlinkUpdate)) {
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
func handleLink(update netlink.LinkUpdate, callback func(supervisor.NetlinkUpdate)) {
	glog.V(3).Infof("[nslistener] link changed: %#v", update)
	if update.IfInfomsg.Flags&syscall.IFF_UP == 1 {
		glog.V(3).Infof("[Link device up]\tupdateLink is:%+v, flag is:0x%x\n", update.Link.Attrs(), update.IfInfomsg.Flags)
	} else {
		if update.Link.Attrs().ParentIndex == 0 {
			glog.V(3).Infof("[Link device !up][Deleted]\tupdateLink is:%+v, flag is:0x%x\n", update.Link.Attrs(), update.IfInfomsg.Flags)
		} else {
			glog.V(3).Infof("[Link device !up]\tupdateLink is:%+v, flag is:0x%x\n", update.Link.Attrs(), update.IfInfomsg.Flags)
		}
	}

	netlinkUpdate := supervisor.NetlinkUpdate{}
	netlinkUpdate.UpdateType = supervisor.UpdateTypeLink
	netlinkUpdate.Link = update

	// We would like to only handle the veth pair link at present.
	if _, ok := (update.Link).(*netlink.Veth); ok {
		callback(netlinkUpdate)
	}
}

// Address specific
func handleAddr(update netlink.AddrUpdate, callback func(supervisor.NetlinkUpdate)) {
	glog.V(3).Infof("[nslistener] address changed: %#v", update)
	if update.NewAddr {
		glog.V(3).Infof("[Add a address]")
	} else {
		glog.V(3).Infof("[Delete a address]")
	}

	if update.LinkAddress.IP.To4() != nil {
		glog.V(3).Infof("[IPv4]\t%+v\n", update)
	} else {
		// We would not like to handle IPv6 at present.
		glog.V(3).Infof("[IPv6]\t%+v\n", update)
		return
	}

	netlinkUpdate := supervisor.NetlinkUpdate{}
	netlinkUpdate.Addr = update
	netlinkUpdate.UpdateType = supervisor.UpdateTypeAddr
	links, err := netlink.LinkList()
	if err != nil {
		glog.Error(err)
	}
	for _, link := range links {
		if link.Attrs().Index == update.LinkIndex && link.Type() == "veth" {
			netlinkUpdate.Link.Link = link
			break
		}
	}
	callback(netlinkUpdate)
}

// Route specific
func handleRoute(update netlink.RouteUpdate, callback func(supervisor.NetlinkUpdate)) {
	glog.V(3).Infof("[nslistener] route changed: %#v", update)
	// Route type is not a bit mask for a couple of values, but a single
	// unsigned int, that's why we use switch here not the "&" operator.
	switch update.Type {
	case syscall.RTM_NEWROUTE:
		glog.V(3).Infof("[Create a route]\t%+v\n", update)
	case syscall.RTM_DELROUTE:
		glog.V(3).Infof("[Remove a route]\t%+v\n", update)
	case syscall.RTM_GETROUTE:
		fmt.Printf("[Receive info of a route]\t%+v\n", update)
	}

	// we would like to only handle the gateway change
	if update.Route.Dst != nil {
		fmt.Println("[Route] is not a gateway, skip")
		return
	}

	netlinkUpdate := supervisor.NetlinkUpdate{}
	netlinkUpdate.Route = update
	netlinkUpdate.UpdateType = supervisor.UpdateTypeRoute
	callback(netlinkUpdate)

	// delete all the routes from outside
	// The reason that we need to delete all the routes is
	// that if we don't do this, we will get two active network interfaces,
	// and ping the IP will get duplicate ICMP replies.
	routes, err := netlink.RouteList(nil, netlink.FAMILY_V4)
	if err != nil {
		fmt.Printf("[Get RouteList] Error:%v\n", err)
		return
	}

	for _, r := range routes {
		if err := netlink.RouteDel(&r); err != nil {
			fmt.Printf("[Remove route] Error:%v\n", err)
		} else {
			fmt.Printf("[Remove route] route:%+v\n", r)
		}
	}
}
