package vfs

import (
	"fmt"
	"os"
	"testing"
)

func TestIsError(t *testing.T) {
	tests := []struct {
		name    string
		errWant error
		errGot  error
		want    bool
	}{
		{"exact match", ErrExist, ErrExist, true},
		{"indirect match", ErrExist, &PathError{Cause: ErrExist}, true},
		{"multi-indirect match", ErrExist, &PathError{Cause: &PathError{Cause: ErrExist}}, true},
		{"no direct match", ErrExist, ErrNotExist, false},
		{"no indirect match", ErrExist, &PathError{Cause: ErrNotExist}, false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := IsError(test.errWant, test.errGot)
			if test.want != got {
				t.Errorf("Wanted %v got %v", test.want, got)
			}
		})
	}
}

func TestIsExist(t *testing.T) {
	tests := []struct {
		name  string
		input error
		check func(error) bool
		want  bool
	}{
		{"IsExist(ErrExist)", ErrExist, IsExist, true},
		{"IsExist(os.ErrExist)", os.ErrExist, IsExist, true},
		{"IsExist(ErrNotExist)", ErrNotExist, IsExist, false},
		{"IsNotExist(ErrNotExist)", ErrNotExist, IsNotExist, true},
		{"IsNotExist(os.ErrNotExist)", os.ErrNotExist, IsNotExist, true},
		{"IsNotExist(ErrExist)", ErrExist, IsNotExist, false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := test.check(test.input)
			if test.want != got {
				t.Errorf("Wanted %v got %v", test.want, got)
			}
		})
	}
}

func TestPathErrorString(t *testing.T) {
	err := &PathError{Op: "mkdir", Path: "/foo/bar", Cause: ErrNotExist}
	want := fmt.Sprintf("mkdir /foo/bar: no such file or directory")
	if err.Error() != want {
		t.Errorf("Wanted error string %q got %q", want, err.Error())
	}
}
