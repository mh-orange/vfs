package vfs

import (
	"fmt"
	"testing"
)

func TestOpenFlagCheck(t *testing.T) {
	tests := []struct {
		flag OpenFlag
		want error
	}{
		{O_RDONLY, nil},
		{O_WRONLY, nil},
		{O_RDWR, nil},
		{O_WRONLY | O_RDWR, ErrInvalidFlags},
		{O_RDONLY | O_APPEND, ErrInvalidFlags},
		{O_RDONLY | O_CREATE, ErrInvalidFlags},
		{O_RDONLY | O_EXCL, ErrInvalidFlags},
		{O_RDONLY | O_SYNC, ErrInvalidFlags},
		{O_RDONLY | O_TRUNC, ErrInvalidFlags},
		{O_RDONLY | O_APPEND | O_CREATE, ErrInvalidFlags},
		{O_RDWR | O_APPEND, nil},
		{O_RDWR | O_CREATE, nil},
		{O_RDWR | O_EXCL, nil},
		{O_RDWR | O_SYNC, nil},
		{O_RDWR | O_TRUNC, nil},
		{O_WRONLY | O_APPEND, nil},
		{O_WRONLY | O_CREATE, nil},
		{O_WRONLY | O_EXCL, nil},
		{O_WRONLY | O_SYNC, nil},
		{O_WRONLY | O_TRUNC, nil},
	}

	for i, test := range tests {
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			got := test.flag.check()
			if test.want != got {
				t.Errorf("Wanted %v got %v", test.want, got)
			}
		})
	}
}
