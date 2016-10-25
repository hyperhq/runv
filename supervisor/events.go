package supervisor

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/golang/glog"
)

const (
	EventExit           = "exit"
	EventContainerStart = "start-container"
	EventProcessStart   = "start-process"
)

var (
	defaultEventsBufferSize = 128
)

type Event struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"`
	Timestamp time.Time `json:"timestamp"`
	PID       string    `json:"pid,omitempty"`
	Status    int       `json:"status,omitempty"`
}

// TODO: copied code, including two bugs
// eventLog is not protected
// Events() might be deadlocked

type SvEvents struct {
	subscriberLock sync.RWMutex
	subscribers    map[chan Event]struct{}

	eventLog  []Event
	eventLock sync.Mutex
}

func (se *SvEvents) setupEventLog(logDir string) error {
	if err := se.readEventLog(logDir); err != nil {
		return err
	}
	events := se.Events(time.Time{}, false, "")
	f, err := os.OpenFile(filepath.Join(logDir, "events.log"), os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0755)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(f)
	go func() {
		for e := range events {
			glog.Infof("write event log: %v", e)
			se.eventLock.Lock()
			se.eventLog = append(se.eventLog, e)
			se.eventLock.Unlock()
			if err := enc.Encode(e); err != nil {
				glog.Infof("containerd: fail to write event to journal")
			}
		}
	}()
	return nil
}

// Note: no locking - don't call after initialization
func (se *SvEvents) readEventLog(logDir string) error {
	f, err := os.Open(filepath.Join(logDir, "events.log"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer f.Close()
	dec := json.NewDecoder(f)
	for {
		var e Event
		if err := dec.Decode(&e); err != nil {
			if err == io.EOF {
				break
			}
			return err
		}
		se.eventLog = append(se.eventLog, e)
	}
	return nil
}

// Events returns an event channel that external consumers can use to receive updates
// on container events
func (se *SvEvents) Events(from time.Time, storedOnly bool, id string) chan Event {
	c := make(chan Event, defaultEventsBufferSize)

	if storedOnly {
		defer se.Unsubscribe(c)
	}

	// Do not allow the subscriber to unsubscript
	se.subscriberLock.Lock()
	defer se.subscriberLock.Unlock()

	if !from.IsZero() {
		// replay old event
		// note: we lock and make a copy of history to avoid blocking
		se.eventLock.Lock()
		past := se.eventLog[:]
		se.eventLock.Unlock()

		for _, e := range past {
			if e.Timestamp.After(from) {
				if id == "" || e.ID == id {
					c <- e
				}
			}
		}

		if storedOnly {
			close(c)
		} else {
			// Notify the client that from now on it's live events
			c <- Event{
				Type:      "live",
				Timestamp: time.Now(),
			}
			se.subscribers[c] = struct{}{}
		}
	}
	return c
}

// Unsubscribe removes the provided channel from receiving any more events
func (se *SvEvents) Unsubscribe(sub chan Event) {
	se.subscriberLock.Lock()
	defer se.subscriberLock.Unlock()
	if _, ok := se.subscribers[sub]; ok {
		delete(se.subscribers, sub)
		close(sub)
	}
}

// notifySubscribers will send the provided event to the external subscribers
// of the events channel
func (se *SvEvents) notifySubscribers(e Event) {
	glog.Infof("notifySubscribers: %v", e)
	se.subscriberLock.RLock()
	defer se.subscriberLock.RUnlock()
	for sub := range se.subscribers {
		// do a non-blocking send for the channel
		select {
		case sub <- e:
		default:
			glog.Warningf("containerd: event not sent to subscriber")
		}
	}
}
