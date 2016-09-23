package qemu

const (
	QMP_INIT = iota
	QMP_SESSION
	QMP_FINISH
	QMP_EVENT
	QMP_INTERNAL_ERROR
	QMP_QUIT
	QMP_TIMEOUT
	QMP_RESULT
	QMP_ERROR
)

const (
	QmpSockName         = "qmp.sock"
	QemuPidFile         = "pidfile"
	QemuLogDir          = "/var/log/hyper/qemu"
	QemuDefaultVsockCid = 1025

	QMP_EVENT_SHUTDOWN = "SHUTDOWN"
)
