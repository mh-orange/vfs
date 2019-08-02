//go:generate stringer -type=EventType

package vfs

import (
	"fmt"
	"path"
	"strings"
	"sync"

	"github.com/fsnotify/fsnotify"
)

type EventType uint32

func (wm EventType) matches(other EventType) bool {
	return wm&other == other
}

const (
	CreateEvent EventType = 1 << iota
	ModifyEvent
	RemoveEvent
	RenameEvent
	AttributeEvent
	ErrorEvent
)

type Event struct {
	Type  EventType
	Path  string
	Error error
}

func (event *Event) String() string {
	return fmt.Sprintf("%s %s %s", path.Dir(event.Path), event.Type, path.Base(event.Path))
}

type Watcher interface {
	Watch(path string) error
	Remove(path string) error
	Close() error
}

type memWatcher struct {
	sync.Mutex
	fs     *memfs
	paths  map[string]struct{}
	events chan<- Event
}

func (mw *memWatcher) Watch(path string) error {
	mw.Lock()
	defer mw.Unlock()
	err := mw.fs.watch(mw, path)
	if err == nil {
		mw.paths[path] = struct{}{}
	}
	return err
}

func (mw *memWatcher) Remove(path string) error {
	mw.Lock()
	defer mw.Unlock()
	delete(mw.paths, path)
	return mw.fs.removeWatch(mw, path)
}

func (mw *memWatcher) Close() error {
	mw.Lock()
	defer mw.Unlock()
	for path := range mw.paths {
		// ignore the error because we don't care if a path is
		// not found
		mw.fs.removeWatch(mw, path)
	}
	close(mw.events)
	return nil
}

type osWatcher struct {
	fs      *osfs
	watcher *fsnotify.Watcher
	events  chan<- Event
	closer  chan bool
}

func (osw *osWatcher) eventLoop() {
	for e := range osw.watcher.Events {
		event := Event{
			Path: strings.TrimPrefix(e.Name, osw.fs.root),
		}
		switch e.Op {
		case fsnotify.Create:
			event.Type = CreateEvent
		case fsnotify.Write:
			event.Type = ModifyEvent
		case fsnotify.Remove:
			event.Type = RemoveEvent
		case fsnotify.Rename:
			event.Type = RenameEvent
		case fsnotify.Chmod:
			event.Type = AttributeEvent
		}
		osw.events <- event
	}
	osw.closer <- true
}

func (osw *osWatcher) errorLoop() {
	for err := range osw.watcher.Errors {
		if err != nil {
			osw.events <- Event{Error: err, Type: ErrorEvent}
		}
	}
	osw.closer <- true
}

func (osw *osWatcher) Remove(path string) error {
	return osw.watcher.Remove(osw.fs.path(path))
}

func (osw *osWatcher) Watch(path string) error {
	return osw.watcher.Add(osw.fs.path(path))
}

func (osw *osWatcher) Close() error {
	err := osw.watcher.Close()
	if err == nil {
		<-osw.closer
		<-osw.closer
		close(osw.events)
	}
	return err
}
