package proxy

import (
	"syscall"

	"github.com/golang/glog"
	"github.com/hyperhq/runv/supervisor"
	"github.com/vishvananda/netlink"
)

type NsListener struct {
	notifyChan chan supervisor.NetlinkUpdate
	doneChan   chan struct{}
}

func SetupNsListener() *NsListener {
	nl := &NsListener{
		notifyChan: make(chan supervisor.NetlinkUpdate, 256),
		doneChan:   make(chan struct{}),
	}
	go nl.Listen()
	return nl
}

// This function should be put into the main process or somewhere that can be
// use to init the network namespace trap.
func (nl *NsListener) Listen() {
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
		case <-nl.doneChan:
			// stop listening
			return
		case updateLink := <-chLink:
			nl.handleLink(updateLink)
		case updateAddr := <-chAddr:
			nl.handleAddr(updateAddr)
		case updateRoute := <-chRoute:
			nl.handleRoute(updateRoute)
		}
	}
}

func (nl *NsListener) StopListen() {
	close(nl.notifyChan)
	close(nl.doneChan)
}

// Link specific
func (nl *NsListener) handleLink(update netlink.LinkUpdate) {
	if update.IfInfomsg.Flags&syscall.IFF_UP == 1 {
		glog.V(3).Infof("[Link device up]updateLink is:%#v, flag is:0x%x", update.Link.Attrs(), update.IfInfomsg.Flags)
	} else {
		if update.Link.Attrs().ParentIndex == 0 {
			glog.V(3).Infof("[Link device !up][Deleted]updateLink is:%#v, flag is:0x%x", update.Link.Attrs(), update.IfInfomsg.Flags)
		} else {
			glog.V(3).Infof("[Link device !up]updateLink is:%#v, flag is:0x%x", update.Link.Attrs(), update.IfInfomsg.Flags)
		}
	}

	netlinkUpdate := supervisor.NetlinkUpdate{}
	netlinkUpdate.UpdateType = supervisor.UpdateTypeLink

	// We would like to only handle the veth pair link at present.
	if veth, ok := (update.Link).(*netlink.Veth); ok {
		netlinkUpdate.Veth = veth
		nl.notifyChan <- netlinkUpdate
	}
}

// Address specific
func (nl *NsListener) handleAddr(update netlink.AddrUpdate) {
	if update.NewAddr {
		glog.V(3).Infof("[Add an address]")
	} else {
		glog.V(3).Infof("[Delete an address]")
	}

	if update.LinkAddress.IP.To4() != nil {
		glog.V(3).Infof("[IPv4]%#v", update)
	} else {
		// We would not like to handle IPv6 at present.
		glog.V(3).Infof("[IPv6]%#v", update)
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
			netlinkUpdate.Veth = link.(*netlink.Veth)
			break
		}
	}
	nl.notifyChan <- netlinkUpdate
}

// Route specific
func (nl *NsListener) handleRoute(update netlink.RouteUpdate) {
	// Route type is not a bit mask for a couple of values, but a single
	// unsigned int, that's why we use switch here not the "&" operator.
	switch update.Type {
	case syscall.RTM_NEWROUTE:
		glog.V(3).Infof("[Create a route]\t%+v\n", update)
	case syscall.RTM_DELROUTE:
		glog.V(3).Infof("[Remove a route]\t%+v\n", update)
	case syscall.RTM_GETROUTE:
		glog.V(3).Infof("[Receive info of a route]\t%+v\n", update)
	}

	netlinkUpdate := supervisor.NetlinkUpdate{}
	netlinkUpdate.Route = update
	netlinkUpdate.UpdateType = supervisor.UpdateTypeRoute
	nl.notifyChan <- netlinkUpdate
}

func (nl *NsListener) GetNotifyChan() chan supervisor.NetlinkUpdate {
	return nl.notifyChan
}
