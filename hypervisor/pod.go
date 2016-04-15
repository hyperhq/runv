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
	Handle func(*types.VmResponse, interface{}, *PodStatus, *Vm) (bool, bool)
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

// Vm DataStructure
type VmVolumeDescriptor struct {
	Device       string `json:"device"`
	Addr         string `json:"addr,omitempty"`
	Mount        string `json:"mount"`
	Fstype       string `json:"fstype,omitempty"`
	ReadOnly     bool   `json:"readOnly"`
	DockerVolume bool   `json:"dockerVolume"`
}

type VmFsmapDescriptor struct {
	Source       string `json:"source"`
	Path         string `json:"path"`
	ReadOnly     bool   `json:"readOnly"`
	DockerVolume bool   `json:"dockerVolume"`
}

type VmEnvironmentVar struct {
	Env   string `json:"env"`
	Value string `json:"value"`
}

type VmProcess struct {
	// Terminal creates an interactive terminal for the process.
	Terminal bool `json:"terminal"`
	// Sequeue number for stdin and stdout
	Stdio uint64 `json:"stdio,omitempty"`
	// sequeue number for stderr if it is not shared with stdout
	Stderr uint64 `json:"stderr,omitempty"`
	// Args specifies the binary and arguments for the application to execute.
	Args []string `json:"args"`
	// Envs populates the process environment for the process.
	Envs []VmEnvironmentVar `json:"envs,omitempty"`
	// Workdir is the current working directory for the process and must be
	// relative to the container's root.
	Workdir string `json:"workdir"`
}

type VmContainer struct {
	Id            string               `json:"id"`
	Rootfs        string               `json:"rootfs"`
	Fstype        string               `json:"fstype,omitempty"`
	Image         string               `json:"image"`
	Addr          string               `json:"addr,omitempty"`
	Volumes       []VmVolumeDescriptor `json:"volumes,omitempty"`
	Fsmap         []VmFsmapDescriptor  `json:"fsmap,omitempty"`
	Sysctl        map[string]string    `json:"sysctl,omitempty"`
	Process       VmProcess            `json:"process"`
	Entrypoint    []string             `json:"-"`
	RestartPolicy string               `json:"restartPolicy"`
	Initialize    bool                 `json:"initialize"`
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
	Dns        []string       `json:"dns,omitempty"`
	Routes     []VmRoute      `json:"routes,omitempty"`
	ShareDir   string         `json:"shareDir"`
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
	var response *types.VmResponse

	Status, err := vm.GetResponseChan()
	if err != nil {
		return nil
	}
	defer vm.ReleaseResponseChan(Status)

	getPodIPEvent := &GetPodIPCommand{
		Id: mypod.Vm,
	}
	vm.Hub <- getPodIPEvent
	// wait for the VM response
	for {
		response = <-Status
		glog.V(1).Infof("Got response, Code %d, VM id %s!", response.Code, response.VmId)
		if response.Reply != getPodIPEvent {
			continue
		}
		if response.VmId == vm.Id {
			break
		}
	}
	if response.Data == nil {
		return []string{}
	}
	return response.Data.([]string)
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
