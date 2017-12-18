// +build libvirt.1.2.14

package libvirt

import (
	"testing"
)

func TestDomainListAllInterfaceAddresses(t *testing.T) {
	dom, conn := buildTestQEMUDomain()
	defer func() {
		dom.Free()
		if res, _ := conn.CloseConnection(); res != 0 {
			t.Errorf("CloseConnection() == %d, expected 0", res)
		}
	}()
	if err := dom.Create(); err != nil {
		t.Error(err)
		return
	}
	defer dom.Destroy()
	ifaces, err := dom.ListAllInterfaceAddresses(0)
	if err != nil {
		t.Fatal(err)
	}

	if len(ifaces) != 0 {
		t.Fatal("should have 0 interfaces", len(ifaces))
	}
}

func TestDomainBlockCopy(t *testing.T) {
	conn := buildTestQEMUConnection()
	// defer conn.CloseConnection()
	defer func() {
		conn.CloseConnection()
	}()

	var disk = "/var/lib/libvirt/images/test-src.qcow2"
	dom, err := conn.DomainCreateXML(
		`<domain type="qemu">
			<name>test</name>
			<memory unit="KiB">8192</memory>
			<os>
				<type>hvm</type>
			</os>
			<devices>
				<disk type='file' device='disk'>
					<source file='`+disk+`'/>
					<target dev='hda'/>
				</disk>
			</devices>
		</domain>`,
		VIR_DOMAIN_NONE)
	if err != nil {
		t.Fatalf("DomainCreateXML: %s", err.Error())
	}
	defer dom.Free()
	defer dom.Destroy()

	var (
		stop = make(chan struct{})
		cb DomainEventCallback = func(c *VirConnection, d *VirDomain, event interface{}, f func()) int {
			if blockJobEvent, ok := event.(DomainBlockJobEvent); ok {
				if blockJobEvent.Type == VIR_DOMAIN_BLOCK_JOB_TYPE_COPY {
					switch blockJobEvent.Status {
					case VIR_DOMAIN_BLOCK_JOB_READY:
						err = d.BlockJobAbort(disk, VIR_DOMAIN_BLOCK_JOB_ABORT_ASYNC)
						if err != nil {
							close(stop)
							t.Fatalf("BlockJobAbort: %s", err.Error())
						}
					case VIR_DOMAIN_BLOCK_JOB_COMPLETED:
						info, err := dom.GetBlockJobInfo(disk, VIR_DOMAIN_BLOCK_JOB_INFO_BANDWIDTH_BYTES)
						if err != nil {
							close(stop)
							t.Fatalf("GetBlockJobInfo: %s", err.Error())
						} else {
							if info.Cur() != info.End() {
								close(stop)
								t.Fatalf("GetBlockJobInfo: Cur != End")
							}
						}
						close(stop)
					default:
						// nothing
					}
				}

			}

			return 0
		}
	)

	if rc := conn.DomainEventRegister(dom, VIR_DOMAIN_EVENT_ID_BLOCK_JOB, &cb, func() {}); rc == -1 {
		t.Fatalf("DomainEventRegister: %s", GetLastError().Error())
	} else {
		defer conn.DomainEventDeregister(rc)
	}

	go func() {
		for {
			select {
			case <-stop:
				return
			default:
				if err = EventRunDefaultImpl(); err != nil {
					close(stop)
					t.Fatalf("EventRunDefaultImpl: %s", err.Error())
					return
				}
			}
		}
	}()

	err = dom.BlockCopy(
		disk,
		`<disk type = 'file'>
			<driver type='qcow2'/>
	  		<source file='/var/lib/libvirt/images/test-dst.qcow2'/>
		</disk>`,
		VirTypedParameters{
			VirTypedParameter{Name: VIR_DOMAIN_BLOCK_COPY_BANDWIDTH, Value: uint64(2147483648)},
			VirTypedParameter{Name: VIR_DOMAIN_BLOCK_COPY_BUF_SIZE, Value: uint64(0)},
			VirTypedParameter{Name: VIR_DOMAIN_BLOCK_COPY_GRANULARITY, Value: uint(512)},
		},
		VIR_DOMAIN_BLOCK_COPY_SHALLOW)
	if err != nil {
		close(stop)
		t.Fatalf("BlockCopy: %s", err.Error())
	}

	<-stop

	return
}
