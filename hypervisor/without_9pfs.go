// +build linux,amd64,!with_9p

package hypervisor

func Is9pfsSupported() bool {
	return false
}
