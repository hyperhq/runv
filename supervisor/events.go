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
	sync.RWMutex
	subscribers map[chan Event]struct{}
	eventLog    []Event
}

func (se *SvEvents) setupEventLog(logDir string) error {
	if err := se.readEventLog(logDir); err != nil {
		return err
	}
	events := se.Events(time.Time{})
	f, err := os.OpenFile(filepath.Join(logDir, "events.log"), os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0755)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(f)
	go func() {
		for e := range events {
			glog.V(3).Infof("write event log: %v", e)
			se.eventLog = append(se.eventLog, e)
			if err := enc.Encode(e); err != nil {
				glog.Errorf("containerd fail to write event to journal: %v", err)
			}
		}
	}()
	return nil
}

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
func (se *SvEvents) Events(from time.Time) chan Event {
	se.Lock()
	defer se.Unlock()
	c := make(chan Event, defaultEventsBufferSize)
	se.subscribers[c] = struct{}{}
	if !from.IsZero() {
		// replay old event
		for _, e := range se.eventLog {
			if e.Timestamp.After(from) {
				c <- e
			}
		}
		// Notify the client that from now on it's live events
		c <- Event{
			Type:      "live",
			Timestamp: time.Now(),
		}
	}
	return c
}

// Unsubscribe removes the provided channel from receiving any more events
func (se *SvEvents) Unsubscribe(sub chan Event) {
	se.Lock()
	defer se.Unlock()
	delete(se.subscribers, sub)
	close(sub)
}

// notifySubscribers will send the provided event to the external subscribers
// of the events channel
func (se *SvEvents) notifySubscribers(e Event) {
	glog.V(3).Infof("notifySubscribers: %v", e)
	se.RLock()
	defer se.RUnlock()
	for sub := range se.subscribers {
		// do a non-blocking send for the channel
		select {
		case sub <- e:
		default:
			glog.V(3).Infof("containerd: event not sent to subscriber")
		}
	}
}
