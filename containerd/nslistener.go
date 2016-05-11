package main

import (
	"encoding/gob"
	"os"
	"syscall"

	"github.com/golang/glog"
	"github.com/hyperhq/runv/hypervisor/pod"
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

	/* send null interface to containerd */
	nics := []pod.UserInterface{}
	if err := enc.Encode(nics); err != nil {
		glog.Error(err)
		return
	}

	/* TODO: watch network setting */
	var exit string
	if err := dec.Decode(&exit); err != nil {
		glog.Error(err)
	}
}
