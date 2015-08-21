package hypervisor

import (
	"github.com/hyperhq/runv/hypervisor/pod"
	"github.com/hyperhq/runv/hypervisor/types"
	"sync"
)

//change first letter to uppercase and add json tag (thanks GNU sed):
//  gsed -ie 's/^    \([a-z]\)\([a-zA-Z]*\)\( \{1,\}[^ ]\{1,\}.*\)$/    \U\1\E\2\3 `json:"\1\2"`/' pod.go

type HandleEvent struct {
	Handle func(*types.VmResponse, interface{}, *Pod, *Vm) bool
	Data   interface{}
}

type Pod struct {
	Id		string
	Name		string
	Vm		string
	Wg		*sync.WaitGroup
	Ip		string
	Containers	[]*Container
	Status		uint
	Type		string
	RestartPolicy	string
	Autoremove	bool
	Handler		HandleEvent
}

type Container struct {
	Id     string
	Name   string
	PodId  string
	Image  string
	Cmds   []string
	Status uint32
}

// Vm DataStructure
type VmVolumeDescriptor struct {
	Device   string `json:"device"`
	Mount    string `json:"mount"`
	Fstype   string `json:"fstype,omitempty"`
	ReadOnly bool   `json:"readOnly"`
}

type VmFsmapDescriptor struct {
	Source   string `json:"source"`
	Path     string `json:"path"`
	ReadOnly bool   `json:"readOnly"`
}

type VmEnvironmentVar struct {
	Env   string `json:"env"`
	Value string `json:"value"`
}

type VmContainer struct {
	Id            string               `json:"id"`
	Rootfs        string               `json:"rootfs"`
	Fstype        string               `json:"fstype,omitempty"`
	Image         string               `json:"image"`
	Volumes       []VmVolumeDescriptor `json:"volumes,omitempty"`
	Fsmap         []VmFsmapDescriptor  `json:"fsmap,omitempty"`
	Tty           uint64               `json:"tty,omitempty"`
	Workdir       string               `json:"workdir"`
	Entrypoint    []string             `json:"-"`
	Cmd           []string             `json:"cmd"`
	Envs          []VmEnvironmentVar   `json:"envs,omitempty"`
	RestartPolicy string               `json:"restartPolicy"`
}

type VmNetworkInf struct {
	Device    string `json:"device"`
	IpAddress string `json:"ipAddress"`
	NetMask   string `json:"netMask"`
}

type VmRoute struct {
	Dest    string `json:"dest"`
	Gateway string `json:"gateway,omitempty"`
	Device  string `json:"device,omitempty"`
}

type VmPod struct {
	Hostname   string         `json:"hostname"`
	Containers []VmContainer  `json:"containers"`
	Interfaces []VmNetworkInf `json:"interfaces,omitempty"`
	Routes     []VmRoute      `json:"routes,omitempty"`
	ShareDir   string         `json:"shareDir"`
}

type RunningContainer struct {
	Id string `json:"id"`
}

type PreparingItem interface {
	ItemType() string
}

func (mypod *Pod) SetPodContainerStatus(data []uint32) {
	failure := 0
	for i, c := range mypod.Containers {
		if data[i] != 0 {
			failure++
			c.Status = types.S_POD_FAILED
		} else {
			c.Status = types.S_POD_SUCCEEDED
		}
	}
	if failure == 0 {
		mypod.Status = types.S_POD_SUCCEEDED
	} else {
		mypod.Status = types.S_POD_FAILED
	}
}

func (mypod *Pod) SetContainerStatus(status uint32) {
	for _, c := range mypod.Containers {
		c.Status = status
	}
}

func (mypod *Pod) AddContainer(containerId, name, image string, cmds []string, status uint32) {
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

func NewPod(podId string, userPod *pod.UserPod) *Pod {
	return &Pod{
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
