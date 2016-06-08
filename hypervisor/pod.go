package hypervisor

import (
	"sync"
	"time"

	"github.com/docker/docker/daemon/logger"
	"github.com/golang/glog"
	"github.com/hyperhq/runv/hypervisor/pod"
	"github.com/hyperhq/runv/hypervisor/types"
)

//change first letter to uppercase and add json tag (thanks GNU sed):
//  gsed -ie 's/^    \([a-z]\)\([a-zA-Z]*\)\( \{1,\}[^ ]\{1,\}.*\)$/    \U\1\E\2\3 `json:"\1\2"`/' pod.go

type HandleEvent struct {
	Handle func(*types.VmResponse, interface{}, *PodStatus, *Vm) bool
	Data   interface{}
}

type PodStatus struct {
	Id            string
	Name          string
	Vm            string
	Wg            *sync.WaitGroup
	Containers    []*Container
	Status        uint
	Type          string
	RestartPolicy string
	Autoremove    bool
	Handler       HandleEvent
	StartedAt     string
	FinishedAt    string
	ResourcePath  string
}

type Container struct {
	Id       string
	Name     string
	PodId    string
	Image    string
	Cmds     []string
	Logs     LogStatus
	Status   uint32
	ExitCode int
}

type LogStatus struct {
	Copier  *logger.Copier
	Driver  logger.Logger
	LogPath string
}

type RunningContainer struct {
	Id string `json:"id"`
}

type PreparingItem interface {
	ItemType() string
}

func (mypod *PodStatus) SetPodContainerStatus(data []uint32) {
	failure := 0
	for i, c := range mypod.Containers {
		if data[i] != 0 {
			failure++
			c.Status = types.S_POD_FAILED
		} else {
			c.Status = types.S_POD_SUCCEEDED
		}
		c.ExitCode = int(data[i])
	}
	if failure == 0 {
		mypod.Status = types.S_POD_SUCCEEDED
	} else {
		mypod.Status = types.S_POD_FAILED
	}
	mypod.FinishedAt = time.Now().Format("2006-01-02T15:04:05Z")
}

func (mypod *PodStatus) SetContainerStatus(status uint32) {
	for _, c := range mypod.Containers {
		c.Status = status
	}
}

func (mypod *PodStatus) SetOneContainerStatus(containerId string, status uint32) {
	for _, c := range mypod.Containers {
		if c.Id == containerId {
			c.Status = status
		}
	}
}

func (mypod *PodStatus) AddContainer(containerId, name, image string, cmds []string, status uint32) {
	container := &Container{
		Id:     containerId,
		Name:   name,
		PodId:  mypod.Id,
		Image:  image,
		Cmds:   cmds,
		Status: status,
	}

	mypod.Containers = append(mypod.Containers, container)
}

func (mypod *PodStatus) GetPodIP(vm *Vm) []string {
	if mypod.Vm == "" {
		return nil
	}

	ips := []string{}

	err := vm.GenericOperation("GetIP", func(ctx *VmContext, result chan<- error) {
		for _, i := range ctx.vmSpec.Interfaces {
			if i.Device == "lo" {
				continue
			}
			ips = append(ips, i.IpAddress)
		}

		result <- nil
	}, StateRunning)

	if err != nil {
		glog.Errorf("get pod ip failed: %v", err)
	}

	return ips
}

func NewPod(podId string, userPod *pod.UserPod) *PodStatus {
	return &PodStatus{
		Id:            podId,
		Name:          userPod.Name,
		Vm:            "",
		Wg:            new(sync.WaitGroup),
		Status:        types.S_POD_CREATED,
		Type:          userPod.Type,
		RestartPolicy: userPod.RestartPolicy,
		Autoremove:    false,
		Handler: HandleEvent{
			Handle: defaultHandlePodEvent,
			Data:   nil,
		},
	}
}
