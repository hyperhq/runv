package libvirt

import (
	"testing"
	"time"
)

func networkXML(netName string) string {
	var name string
	if netName == "" {
		name = time.Now().String()
	} else {
		name = netName
	}

	return `<network>
    <name>` + name + `</name>
    <bridge name="testbr0"/>
    <forward/>
    <ip address="192.168.0.1" netmask="255.255.255.0">
    </ip>
    </network>`
}

func buildTestNetwork(netName string) (VirNetwork, VirConnection) {
	conn := buildTestConnection()
	networkXML := networkXML(netName)
	net, err := conn.NetworkDefineXML(networkXML)
	if err != nil {
		panic(err)
	}
	return net, conn
}

func TestGetNetworkName(t *testing.T) {
	net, conn := buildTestNetwork("")
	defer func() {
		net.Free()
		if res, _ := conn.CloseConnection(); res != 0 {
			t.Errorf("CloseConnection() == %d, expected 0", res)
		}
	}()
	if _, err := net.GetName(); err != nil {
		t.Fatal(err)
		return
	}
}

func TestGetNetworkUUID(t *testing.T) {
	net, conn := buildTestNetwork("")
	defer func() {
		net.Free()
		if res, _ := conn.CloseConnection(); res != 0 {
			t.Errorf("CloseConnection() == %d, expected 0", res)
		}
	}()
	_, err := net.GetUUID()
	if err != nil {
		t.Error(err)
		return
	}
}

func TestGetNetworkUUIDString(t *testing.T) {
	net, conn := buildTestNetwork("")
	defer func() {
		net.Free()
		if res, _ := conn.CloseConnection(); res != 0 {
			t.Errorf("CloseConnection() == %d, expected 0", res)
		}
	}()
	_, err := net.GetUUIDString()
	if err != nil {
		t.Error(err)
		return
	}
}

func TestGetNetworkXMLDesc(t *testing.T) {
	net, conn := buildTestNetwork("")
	defer func() {
		net.Free()
		if res, _ := conn.CloseConnection(); res != 0 {
			t.Errorf("CloseConnection() == %d, expected 0", res)
		}
	}()
	if _, err := net.GetXMLDesc(0); err != nil {
		t.Error(err)
		return
	}
}

func TestCreateDestroyNetwork(t *testing.T) {
	net, conn := buildTestNetwork("")
	defer func() {
		net.Free()
		if res, _ := conn.CloseConnection(); res != 0 {
			t.Errorf("CloseConnection() == %d, expected 0", res)
		}
	}()
	if err := net.Create(); err != nil {
		t.Error(err)
		return
	}

	if err := net.Destroy(); err != nil {
		t.Error(err)
		return
	}
}

func TestNetworkAutostart(t *testing.T) {
	net, conn := buildTestNetwork("")
	defer func() {
		net.Free()
		if res, _ := conn.CloseConnection(); res != 0 {
			t.Errorf("CloseConnection() == %d, expected 0", res)
		}
	}()
	as, err := net.GetAutostart()
	if err != nil {
		t.Error(err)
		return
	}
	if as {
		t.Fatal("autostart should be false")
		return
	}
	if err := net.SetAutostart(true); err != nil {
		t.Error(err)
		return
	}
	as, err = net.GetAutostart()
	if err != nil {
		t.Error(err)
		return
	}
	if !as {
		t.Fatal("autostart should be true")
		return
	}
}

func TestNetworkIsActive(t *testing.T) {
	net, conn := buildTestNetwork("")
	defer func() {
		net.Free()
		if res, _ := conn.CloseConnection(); res != 0 {
			t.Errorf("CloseConnection() == %d, expected 0", res)
		}
	}()
	if err := net.Create(); err != nil {
		t.Log(err)
		return
	}
	active, err := net.IsActive()
	if err != nil {
		t.Error(err)
		return
	}
	if !active {
		t.Fatal("Network should be active")
		return
	}
	if err := net.Destroy(); err != nil {
		t.Error(err)
		return
	}
	active, err = net.IsActive()
	if err != nil {
		t.Error(err)
		return
	}
	if active {
		t.Fatal("Network should be inactive")
		return
	}
}

func TestNetworkGetBridgeName(t *testing.T) {
	net, conn := buildTestNetwork("")
	defer func() {
		net.Free()
		if res, _ := conn.CloseConnection(); res != 0 {
			t.Errorf("CloseConnection() == %d, expected 0", res)
		}
	}()
	if err := net.Create(); err != nil {
		t.Error(err)
		return
	}
	brName := "testbr0"
	br, err := net.GetBridgeName()
	if err != nil {
		t.Errorf("got %s but expected %s", br, brName)
	}
}

func TestNetworkFree(t *testing.T) {
	net, conn := buildTestNetwork("")
	defer func() {
		if res, _ := conn.CloseConnection(); res != 0 {
			t.Errorf("CloseConnection() == %d, expected 0", res)
		}
	}()
	if err := net.Free(); err != nil {
		t.Error(err)
		return
	}
}

func TestNetworkCreateXML(t *testing.T) {
	conn := buildTestConnection()
	networkXML := networkXML("")
	net, err := conn.NetworkCreateXML(networkXML)
	if err != nil {
		t.Error(err)
	}
	defer func() {
		net.Free()
		if res, _ := conn.CloseConnection(); res != 0 {
			t.Errorf("CloseConnection() == %d, expected 0", res)
		}
	}()

	if is_active, err := net.IsActive(); err != nil {
		t.Error(err)
	} else {
		if !is_active {
			t.Error("Network should be active")
		}
	}
	if is_persistent, err := net.IsPersistent(); err != nil {
		t.Error(err)
	} else {
		if is_persistent {
			t.Error("Network should not be persistent")
		}
	}
}
