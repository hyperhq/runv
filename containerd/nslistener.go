package main

import (
	"encoding/gob"
	"os"
	"syscall"

	"github.com/golang/glog"
	"github.com/hyperhq/runv/supervisor"
	"github.com/vishvananda/netlink"
)

func nsListenerDaemon() {
	/* create own netns */
	if err := syscall.Unshare(syscall.CLONE_NEWNET); err != nil {
		glog.Error(err)
		return
	}

	childPipe := os.NewFile(uintptr(3), "child")
	enc := gob.NewEncoder(childPipe)
	dec := gob.NewDecoder(childPipe)

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

	/* send interface info to containerd */
	infos := collectionInterfaceInfo()
	if err := enc.Encode(infos); err != nil {
		glog.Error(err)
		return
	}

	/* TODO: watch network setting */
	var exit string
	if err := dec.Decode(&exit); err != nil {
		glog.Error(err)
	}
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

		addrs, err := netlink.AddrList(link, netlink.FAMILY_V4)
		if err != nil {
			glog.Error(err)
			return infos
		}

		for _, addr := range addrs {
			info := supervisor.InterfaceInfo{
				Ip:        addr.IPNet.String(),
				Index:     link.Attrs().Index,
				PeerIndex: link.Attrs().ParentIndex,
			}
			glog.Infof("get interface %v", info)
			infos = append(infos, info)
		}
	}
	return infos
}
