package vfs

import (
	"testing"

	"github.com/fsnotify/fsnotify"
)

func TestWatcherEventTypeMatch(t *testing.T) {
	tests := []struct {
		name  string
		match EventType
		other EventType
		want  bool
	}{
		{"CreateEvent (match)", CreateEvent, CreateEvent, true},
		{"CreateEvent (no match)", CreateEvent, ModifyEvent, false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := test.match.matches(test.other)
			if test.want != got {
				t.Errorf("Wanted %v got %v", test.want, got)
			}
		})
	}
}

func TestWatcherEventString(t *testing.T) {
	tests := []struct {
		name  string
		event *Event
		want  string
	}{
		{"CreateEvent", &Event{CreateEvent, "/dir/file", nil}, "/dir CreateEvent file"},
		{"ModifyEvent", &Event{ModifyEvent, "/dir/file", nil}, "/dir ModifyEvent file"},
		{"RemoveEvent", &Event{RemoveEvent, "/dir/file", nil}, "/dir RemoveEvent file"},
		{"RenameEvent", &Event{RenameEvent, "/dir/file", nil}, "/dir RenameEvent file"},
		{"AttributeEvent", &Event{AttributeEvent, "/dir/file", nil}, "/dir AttributeEvent file"},
		{"ErrorEvent", &Event{ErrorEvent, "/dir/file", nil}, "/dir ErrorEvent file"},
		{"UnknownEvent", &Event{EventType(128), "/dir/file", nil}, "/dir EventType(128) file"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := test.event.String()
			if test.want != got {
				t.Errorf("Wanted %v got %v", test.want, got)
			}
		})
	}
}

func TestWatcherOsEventLoop(t *testing.T) {
	tests := []struct {
		name string
		root string
		op   fsnotify.Op
		path string
		err  error
		want Event
	}{
		{"Create", "/foobar", fsnotify.Create, "/foobar/hello/world.txt", nil, Event{CreateEvent, "/hello/world.txt", nil}},
		{"Write", "/foobar", fsnotify.Write, "/foobar/hello/world.txt", nil, Event{ModifyEvent, "/hello/world.txt", nil}},
		{"Remove", "/foobar", fsnotify.Remove, "/foobar/hello/world.txt", nil, Event{RemoveEvent, "/hello/world.txt", nil}},
		{"Rename", "/foobar", fsnotify.Rename, "/foobar/hello/world.txt", nil, Event{RenameEvent, "/hello/world.txt", nil}},
		{"Chmod", "/foobar", fsnotify.Chmod, "/foobar/hello/world.txt", nil, Event{AttributeEvent, "/hello/world.txt", nil}},
		{"Error", "", fsnotify.Chmod, "", ErrIsDir, Event{ErrorEvent, "", ErrIsDir}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fs := NewOsFs(test.root).(*osfs)
			events := make(chan Event, 1)
			w, err := fs.Watcher(events)
			watcher := w.(*osWatcher)
			if err == nil {
				if test.err == nil {
					watcher.watcher.Events <- fsnotify.Event{Name: test.path, Op: test.op}
				} else {
					watcher.watcher.Errors <- test.err
				}
				got := <-events
				if test.want != got {
					t.Errorf("Wanted %v got %v", test.want, got)
				}
			} else {
				t.Errorf("Unexpected error: %v", err)
			}
			watcher.Close()
			fs.Close()
		})
	}
}
