package libvirt

import (
	"strings"
	"testing"
	"time"
)

func buildTestQEMUDomain() (VirDomain, VirConnection) {
	conn := buildTestQEMUConnection()
	dom, err := conn.DomainDefineXML(`<domain type="qemu">
		<name>` + strings.Replace(time.Now().String(), " ", "_", -1) + `</name>
		<memory unit="KiB">128</memory>
		<os>
			<type>hvm</type>
		</os>
	</domain>`)
	if err != nil {
		panic(err)
	}
	return dom, conn
}
func buildTestDomain() (VirDomain, VirConnection) {
	conn := buildTestConnection()
	dom, err := conn.DomainDefineXML(`<domain type="test">
		<name>` + time.Now().String() + `</name>
		<memory unit="KiB">8192</memory>
		<os>
			<type>hvm</type>
		</os>
	</domain>`)
	if err != nil {
		panic(err)
	}
	return dom, conn
}

func buildSMPTestDomain() (VirDomain, VirConnection) {
	conn := buildTestConnection()
	dom, err := conn.DomainDefineXML(`<domain type="test">
		<name>` + time.Now().String() + `</name>
		<memory unit="KiB">8192</memory>
		<vcpu>8</vcpu>
  		<os>
			<type>hvm</type>
		</os>
	</domain>`)
	if err != nil {
		panic(err)
	}
	return dom, conn
}

func buildTransientTestDomain() (VirDomain, VirConnection) {
	conn := buildTestConnection()
	dom, err := conn.DomainCreateXML(`<domain type="test">
		<name>`+time.Now().String()+`</name>
		<memory unit="KiB">8192</memory>
		<os>
			<type>hvm</type>
		</os>
	</domain>`, VIR_DOMAIN_NONE)
	if err != nil {
		panic(err)
	}
	return dom, conn
}

func TestUndefineDomain(t *testing.T) {
	dom, conn := buildTestDomain()
	defer func() {
		dom.Free()
		if res, _ := conn.CloseConnection(); res != 0 {
			t.Errorf("CloseConnection() == %d, expected 0", res)
		}
	}()
	name, err := dom.GetName()
	if err != nil {
		t.Error(err)
		return
	}
	if err := dom.Undefine(); err != nil {
		t.Error(err)
		return
	}
	if _, err := conn.LookupDomainByName(name); err == nil {
		t.Fatal("Shouldn't have been able to find domain")
		return
	}
}

func TestGetDomainName(t *testing.T) {
	dom, conn := buildTestDomain()
	defer func() {
		dom.Undefine()
		dom.Free()
		if res, _ := conn.CloseConnection(); res != 0 {
			t.Errorf("CloseConnection() == %d, expected 0", res)
		}
	}()
	if _, err := dom.GetName(); err != nil {
		t.Error(err)
		return
	}
}

func TestGetDomainState(t *testing.T) {
	dom, conn := buildTestDomain()
	defer func() {
		dom.Free()
		if res, _ := conn.CloseConnection(); res != 0 {
			t.Errorf("CloseConnection() == %d, expected 0", res)
		}
	}()
	state, err := dom.GetState()
	if err != nil {
		t.Error(err)
		return
	}
	if len(state) != 2 {
		t.Error("Length of domain state should be 2")
		return
	}
	if state[0] != 5 || state[1] != 0 {
		t.Error("Domain state in test transport should be [5 0]")
		return
	}
}

func TestGetDomainID(t *testing.T) {
	dom, conn := buildTestDomain()
	defer func() {
		dom.Free()
		if res, _ := conn.CloseConnection(); res != 0 {
			t.Errorf("CloseConnection() == %d, expected 0", res)
		}
	}()

	if err := dom.Create(); err != nil {
		t.Error("Failed to create domain")
	}

	if id, err := dom.GetID(); id == ^uint(0) || err != nil {
		dom.Destroy()
		t.Error("Couldn't get domain ID")
		return
	}
	dom.Destroy()
}

func TestGetDomainUUID(t *testing.T) {
	dom, conn := buildTestDomain()
	defer func() {
		dom.Free()
		if res, _ := conn.CloseConnection(); res != 0 {
			t.Errorf("CloseConnection() == %d, expected 0", res)
		}
	}()
	_, err := dom.GetUUID()
	// how to test uuid validity?
	if err != nil {
		t.Error(err)
		return
	}
}

func TestGetDomainUUIDString(t *testing.T) {
	dom, conn := buildTestDomain()
	defer func() {
		dom.Free()
		if res, _ := conn.CloseConnection(); res != 0 {
			t.Errorf("CloseConnection() == %d, expected 0", res)
		}
	}()
	_, err := dom.GetUUIDString()
	if err != nil {
		t.Error(err)
		return
	}
}

func TestGetDomainInfo(t *testing.T) {
	dom, conn := buildTestDomain()
	defer func() {
		dom.Free()
		if res, _ := conn.CloseConnection(); res != 0 {
			t.Errorf("CloseConnection() == %d, expected 0", res)
		}
	}()
	_, err := dom.GetInfo()
	if err != nil {
		t.Error(err)
		return
	}
}

func TestGetDomainXMLDesc(t *testing.T) {
	dom, conn := buildTestDomain()
	defer func() {
		dom.Free()
		if res, _ := conn.CloseConnection(); res != 0 {
			t.Errorf("CloseConnection() == %d, expected 0", res)
		}
	}()
	_, err := dom.GetXMLDesc(0)
	if err != nil {
		t.Error(err)
		return
	}
}

func TestCreateDomainSnapshotXML(t *testing.T) {
	dom, conn := buildTestDomain()
	defer func() {
		dom.Free()
		if res, _ := conn.CloseConnection(); res != 0 {
			t.Errorf("CloseConnection() == %d, expected 0", res)
		}
	}()
	ss, err := dom.CreateSnapshotXML(`
		<domainsnapshot>
			<description>Test snapshot that will fail because its unsupported</description>
		</domainsnapshot>
	`, 0)
	if err != nil {
		t.Error(err)
		return
	}
	defer ss.Free()
}

func TestSaveDomain(t *testing.T) {
	dom, conn := buildTestDomain()
	defer func() {
		dom.Free()
		if res, _ := conn.CloseConnection(); res != 0 {
			t.Errorf("CloseConnection() == %d, expected 0", res)
		}
	}()
	// get the name so we can get a handle on it later
	domName, err := dom.GetName()
	if err != nil {
		t.Error(err)
		return
	}
	const tmpFile = "/tmp/libvirt-go-test.tmp"
	if err := dom.Save(tmpFile); err != nil {
		t.Error(err)
		return
	}
	if err := conn.Restore(tmpFile); err != nil {
		t.Error(err)
		return
	}
	if dom2, err := conn.LookupDomainByName(domName); err != nil {
		t.Error(err)
		return
	} else {
		dom2.Free()
	}
}

func TestSaveDomainFlags(t *testing.T) {
	dom, conn := buildTestDomain()
	defer func() {
		dom.Free()
		if res, _ := conn.CloseConnection(); res != 0 {
			t.Errorf("CloseConnection() == %d, expected 0", res)
		}
	}()
	const srcFile = "/tmp/libvirt-go-test.tmp"
	if err := dom.SaveFlags(srcFile, "", 0); err == nil {
		t.Fatal("expected xml modification unsupported")
		return
	}
}

func TestCreateDestroyDomain(t *testing.T) {
	dom, conn := buildTestDomain()
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
	state, err := dom.GetState()
	if err != nil {
		t.Error(err)
		return
	}
	if state[0] != VIR_DOMAIN_RUNNING {
		t.Fatal("Domain should be running")
		return
	}
	if err = dom.Destroy(); err != nil {
		t.Error(err)
		return
	}
	state, err = dom.GetState()
	if err != nil {
		t.Error(err)
		return
	}
	if state[0] != VIR_DOMAIN_SHUTOFF {
		t.Fatal("Domain should be destroyed")
		return
	}
}

func TestShutdownDomain(t *testing.T) {
	dom, conn := buildTestDomain()
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
	if err := dom.Shutdown(); err != nil {
		t.Error(err)
		return
	}
	state, err := dom.GetState()
	if err != nil {
		t.Error(err)
		return
	}
	if state[0] != 5 || state[1] != 1 {
		t.Fatal("state should be [5 1]")
		return
	}
}

func TestShutdownReboot(t *testing.T) {
	dom, conn := buildTestDomain()
	defer func() {
		dom.Free()
		if res, _ := conn.CloseConnection(); res != 0 {
			t.Errorf("CloseConnection() == %d, expected 0", res)
		}
	}()
	if err := dom.Reboot(0); err != nil {
		t.Error(err)
		return
	}
}

func TestDomainAutostart(t *testing.T) {
	dom, conn := buildTestDomain()
	defer func() {
		dom.Free()
		if res, _ := conn.CloseConnection(); res != 0 {
			t.Errorf("CloseConnection() == %d, expected 0", res)
		}
	}()
	as, err := dom.GetAutostart()
	if err != nil {
		t.Error(err)
		return
	}
	if as {
		t.Fatal("autostart should be false")
		return
	}
	if err := dom.SetAutostart(true); err != nil {
		t.Error(err)
		return
	}
	as, err = dom.GetAutostart()
	if err != nil {
		t.Error(err)
		return
	}
	if !as {
		t.Fatal("autostart should be true")
		return
	}
}

func TestDomainIsActive(t *testing.T) {
	dom, conn := buildTestDomain()
	defer func() {
		dom.Free()
		if res, _ := conn.CloseConnection(); res != 0 {
			t.Errorf("CloseConnection() == %d, expected 0", res)
		}
	}()
	if err := dom.Create(); err != nil {
		t.Log(err)
		return
	}
	active, err := dom.IsActive()
	if err != nil {
		t.Error(err)
		return
	}
	if !active {
		t.Fatal("Domain should be active")
		return
	}
	if err := dom.Destroy(); err != nil {
		t.Error(err)
		return
	}
	active, err = dom.IsActive()
	if err != nil {
		t.Error(err)
		return
	}
	if active {
		t.Fatal("Domain should be inactive")
		return
	}
}

func TestDomainIsPersistent(t *testing.T) {
	dom, conn := buildTransientTestDomain()
	dom2, conn2 := buildTestDomain()
	defer func() {
		dom.Free()
		dom2.Free()
		if res, _ := conn.CloseConnection(); res != 0 {
			t.Errorf("CloseConnection() == %d, expected 0", res)
		}
		if res, _ := conn2.CloseConnection(); res != 0 {
			t.Errorf("CloseConnection() == %d, expected 0", res)
		}
	}()
	persistent, err := dom.IsPersistent()
	if err != nil {
		t.Error(err)
		return
	}
	if persistent {
		t.Fatal("Domain shouldn't be persistent")
		return
	}
	persistent, err = dom2.IsPersistent()
	if err != nil {
		t.Error(err)
		return
	}
	if !persistent {
		t.Fatal("Domain should be persistent")
		return
	}
}

func TestDomainSetMaxMemory(t *testing.T) {
	const mem = 8192 * 100
	dom, conn := buildTestDomain()
	defer func() {
		dom.Free()
		if res, _ := conn.CloseConnection(); res != 0 {
			t.Errorf("CloseConnection() == %d, expected 0", res)
		}
	}()
	if err := dom.SetMaxMemory(mem); err != nil {
		t.Error(err)
		return
	}
}

func TestDomainSetMemory(t *testing.T) {
	dom, conn := buildTestDomain()
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
	if err := dom.SetMemory(1024); err != nil {
		t.Error(err)
		return
	}
}

func TestDomainSetVcpus(t *testing.T) {
	dom, conn := buildTestDomain()
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
	if err := dom.SetVcpus(1); err != nil {
		t.Error(err)
		return
	}
	if err := dom.SetVcpusFlags(1, VIR_DOMAIN_VCPU_LIVE); err != nil {
		t.Error(err)
		return
	}
}

func TestDomainFree(t *testing.T) {
	dom, conn := buildTestDomain()
	defer func() {
		if res, _ := conn.CloseConnection(); res != 0 {
			t.Errorf("CloseConnection() == %d, expected 0", res)
		}
	}()
	if err := dom.Free(); err != nil {
		t.Error(err)
		return
	}
}

func TestDomainSuspend(t *testing.T) {
	dom, conn := buildTestDomain()
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
	if err := dom.Suspend(); err != nil {
		t.Error(err)
		return
	}
	defer dom.Resume()
}

func TesDomainShutdownFlags(t *testing.T) {
	dom, conn := buildTestDomain()
	defer func() {
		if res, _ := conn.CloseConnection(); res != 0 {
			t.Errorf("CloseConnection() == %d, expected 0", res)
		}
	}()
	if err := dom.Create(); err != nil {
		t.Error(err)
		return
	}
	if err := dom.ShutdownFlags(VIR_DOMAIN_SHUTDOWN_SIGNAL); err != nil {
		t.Error(err)
		return
	}
	state, err := dom.GetState()
	if err != nil {
		t.Error(err)
		return
	}
	if state[0] != 5 || state[1] != 1 {
		t.Fatal("state should be [5 1]")
		return
	}
}

func TesDomainDestoryFlags(t *testing.T) {
	dom, conn := buildTestDomain()
	defer func() {
		if res, _ := conn.CloseConnection(); res != 0 {
			t.Errorf("CloseConnection() == %d, expected 0", res)
		}
	}()
	if err := dom.Create(); err != nil {
		t.Error(err)
		return
	}
	if err := dom.DestroyFlags(VIR_DOMAIN_DESTROY_GRACEFUL); err != nil {
		t.Error(err)
		return
	}
	state, err := dom.GetState()
	if err != nil {
		t.Error(err)
		return
	}
	if state[0] != 5 || state[1] != 1 {
		t.Fatal("state should be [5 1]")
		return
	}
}

func TestDomainScreenshot(t *testing.T) {
	dom, conn := buildTestDomain()
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
	stream, err := NewVirStream(&conn, 0)
	if err != nil {
		t.Fatalf("failed to create new stream: %s", err)
	}
	defer stream.Free()
	mime, err := dom.Screenshot(stream, 0, 0)
	if err != nil {
		t.Fatalf("failed to take screenshot: %s", err)
	}
	if strings.Index(mime, "image/") != 0 {
		t.Fatalf("Wanted image/*, got %s", mime)
	}
}

func TestDomainGetVcpus(t *testing.T) {
	dom, conn := buildTestDomain()
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

	stats, err := dom.GetVcpus(1)
	if err != nil {
		t.Fatal(err)
	}

	if len(stats) != 1 {
		t.Fatal("should have 1 cpu")
	}

	if stats[0].State != 1 {
		t.Fatal("state should be 1")
	}
}

func TestDomainGetVcpusCpuMap(t *testing.T) {
	dom, conn := buildSMPTestDomain()
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

	ni, err := conn.GetNodeInfo()
	if err != nil {
		panic(err)
	}

	stats, err := dom.GetVcpusCpuMap(8, ni.GetMaxCPUs())
	if err != nil {
		t.Fatal(err)
	}

	if len(stats) != 8 {
		t.Fatal("should have 8 cpu")
	}

	if stats[0].State != 1 {
		t.Fatal("state should be 1")
	}
}

func TestDomainGetVcpusFlags(t *testing.T) {
	dom, conn := buildTestDomain()
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

	num, err := dom.GetVcpusFlags(0)
	if err != nil {
		t.Fatal(err)
	}

	if num != 1 {
		t.Fatal("should have 1 cpu", num)
	}
}

func TestDomainPinVcpu(t *testing.T) {
	dom, conn := buildSMPTestDomain()
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

	ni, err := conn.GetNodeInfo()
	if err != nil {
		panic(err)
	}

	err = dom.PinVcpu(2, []uint32{2, 5}, ni.GetMaxCPUs())
	if err != nil {
		t.Fatal(err)
	}
}

func TestQemuMonitorCommand(t *testing.T) {
	dom, conn := buildTestQEMUDomain()
	defer func() {
		dom.Destroy()
		dom.Undefine()
		dom.Free()
		if res, _ := conn.CloseConnection(); res != 0 {
			t.Errorf("CloseConnection() == %d, expected 0", res)
		}
	}()

	if err := dom.Create(); err != nil {
		t.Error(err)
		return
	}

	if _, err := dom.QemuMonitorCommand(VIR_DOMAIN_QEMU_MONITOR_COMMAND_DEFAULT, "{\"execute\" : \"query-cpus\"}"); err != nil {
		t.Error(err)
		return
	}

	if _, err := dom.QemuMonitorCommand(VIR_DOMAIN_QEMU_MONITOR_COMMAND_HMP, "info cpus"); err != nil {
		t.Error(err)
		return
	}
}

func TestDomainCreateWithFlags(t *testing.T) {
	dom, conn := buildTestQEMUDomain()
	defer func() {
		dom.Destroy()
		dom.Undefine()
		dom.Free()
		if res, _ := conn.CloseConnection(); res != 0 {
			t.Errorf("CloseConnection() == %d, expected 0", res)
		}
	}()

	if err := dom.CreateWithFlags(VIR_DOMAIN_START_PAUSED); err != nil {
		state, err := dom.GetState()
		if err != nil {
			t.Error(err)
			return
		}

		if state[0] != VIR_DOMAIN_PAUSED {
			t.Fatalf("Domain should be paused")
		}
	}
}
