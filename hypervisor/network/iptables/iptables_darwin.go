// +build darwin

package iptables

import (
	"errors"
	"fmt"
	"net"
	"os/exec"

	"github.com/hyperhq/runv/lib/glog"
	"github.com/hyperhq/runv/lib/govbox"
)

type Action string
type Table string

const (
	Append Action = "-A"
	Delete Action = "-D"
	Insert Action = "-I"
	Nat    Table  = "nat"
	Filter Table  = "filter"
	Mangle Table  = "mangle"
)

var (
	iptablesPath        string
	supportsXlock       = false
	ErrIptablesNotFound = errors.New("vboxmanage not found")
)

type Chain struct {
	Name   string
	Bridge string
	Table  Table
}

type ChainError struct {
	Chain  string
	Output []byte
}

func (e *ChainError) Error() string {
	return fmt.Sprintf("Error lsof%s: %s", e.Chain, string(e.Output))
}

func initCheck() error {
	if iptablesPath == "" {
		path, err := exec.LookPath("lsof")
		if err != nil {
			return ErrIptablesNotFound
		}
		iptablesPath = path
	}
	return nil
}

// Check if a dnat rule exists
func OperatePortMap(action Action, vmId string, index int, proto string,
	hostport int, containerip string, cport int) error {
	machine, err := virtualbox.GetMachine(vmId)
	if err != nil {
		return fmt.Errorf("Can not find vm machine, %s", err.Error())
	}
	if action == Insert {
		pf := virtualbox.PFRule{
			Proto:     virtualbox.PFProto(proto),
			HostIP:    nil,
			HostPort:  uint16(hostport),
			GuestIP:   net.IP(containerip),
			GuestPort: uint16(cport),
		}

		err = machine.AddNATPF(index+1, fmt.Sprintf("hyper-%d", index), pf)
		if err != nil {
			return fmt.Errorf("Add port forwarding failed, %s", err.Error())
		}
	}
	if action == Delete {
		err = machine.DelNATPF(index+1, fmt.Sprintf("hyper-%d", index))
		if err != nil {
			return fmt.Errorf("Del port forwarding failed, %s", err.Error())
		}
	}

	return nil
}

func PortMapExists(proto, hostport string) bool {
	// lsof -i tcp:80 to check
	cmd := exec.Command(iptablesPath, fmt.Sprintf("-i %s:%s", proto, hostport))
	output, err := cmd.Output()
	if err != nil {
		glog.Errorf("failed to check the portmap, %s", err.Error())
		return false
	}
	if len(string(output)) > 0 {
		return true
	}

	return false
}

func PortMapUsed(chain string, rule []string) bool {
	return false
}

// Check if a rule exists
func Exists(table Table, chain string, rule ...string) bool {
	return false
}
