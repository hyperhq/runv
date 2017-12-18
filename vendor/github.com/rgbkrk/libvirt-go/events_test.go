package libvirt

import (
	"fmt"
	"testing"
	"time"
)

func init() {
	EventRegisterDefaultImpl()
}

func TestDomainEventRegister(t *testing.T) {

	callbackId := -1

	conn := buildTestConnection()
	defer func() {
		if callbackId >= 0 {
			if err := conn.DomainEventDeregister(callbackId); err != nil {
				t.Errorf("got `%v` on DomainEventDeregister instead of nil", err)
			}
		}
		if res, _ := conn.CloseConnection(); res != 0 {
			t.Errorf("CloseConnection() == %d, expected 0", res)
		}
	}()

	nodom := VirDomain{}
	defName := time.Now().String()

	nbEvents := 0

	callback := DomainEventCallback(
		func(c *VirConnection, d *VirDomain, eventDetails interface{}, f func()) int {
			if lifecycleEvent, ok := eventDetails.(DomainLifecycleEvent); ok {
				if lifecycleEvent.Event == VIR_DOMAIN_EVENT_STARTED {
					domName, _ := d.GetName()
					if defName != domName {
						t.Fatalf("Name was not '%s': %s", defName, domName)
					}
				}
				eventString := fmt.Sprintf("%s", lifecycleEvent)
				expected := "Domain event=\"started\" detail=\"booted\""
				if eventString != expected {
					t.Errorf("event == %q, expected %q", eventString, expected)
				}
			} else {
				t.Fatalf("event details isn't DomainLifecycleEvent: %s", eventDetails)
			}
			f()
			return 0
		},
	)

	callbackId = conn.DomainEventRegister(
		VirDomain{},
		VIR_DOMAIN_EVENT_ID_LIFECYCLE,
		&callback,
		func() {
			nbEvents++
		},
	)

	// Test a minimally valid xml
	xml := `<domain type="test">
		<name>` + defName + `</name>
		<memory unit="KiB">8192</memory>
		<os>
			<type>hvm</type>
		</os>
	</domain>`
	dom, err := conn.DomainCreateXML(xml, VIR_DOMAIN_NONE)
	if err != nil {
		t.Error(err)
		return
	}

	// This is blocking as long as there is no message
	EventRunDefaultImpl()
	if nbEvents == 0 {
		t.Fatal("At least one event was expected")
	}

	defer func() {
		if dom != nodom {
			dom.Destroy()
			dom.Free()
		}
	}()

	// Check that the internal context entry was added, and that there only is
	// one.
	goCallbackLock.Lock()
	if len(goCallbacks) != 1 {
		t.Errorf("goCallbacks should hold one entry, got %+v", goCallbacks)
	}
	goCallbackLock.Unlock()

	// Deregister the event
	if err := conn.DomainEventDeregister(callbackId); err != nil {
		t.Fatal("Event deregistration failed with: %v", err)
	}
	callbackId = -1 // Don't deregister twice

	// Check that the internal context entries was removed
	goCallbackLock.Lock()
	if len(goCallbacks) > 0 {
		t.Errorf("goCallbacks entry wasn't removed: %+v", goCallbacks)
	}
	goCallbackLock.Unlock()
}
