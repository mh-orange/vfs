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
		{RdOnlyFlag, nil},
		{WrOnlyFlag, nil},
		{RdWrFlag, nil},
		{WrOnlyFlag | RdWrFlag, ErrInvalidFlags},
		{RdOnlyFlag | AppendFlag, ErrInvalidFlags},
		{RdOnlyFlag | CreateFlag, ErrInvalidFlags},
		{RdOnlyFlag | ExclFlag, ErrInvalidFlags},
		{RdOnlyFlag | TruncFlag, ErrInvalidFlags},
		{RdOnlyFlag | AppendFlag | CreateFlag, ErrInvalidFlags},
		{RdWrFlag | AppendFlag, nil},
		{RdWrFlag | CreateFlag, nil},
		{RdWrFlag | ExclFlag, nil},
		{RdWrFlag, nil},
		{RdWrFlag | TruncFlag, nil},
		{WrOnlyFlag | AppendFlag, nil},
		{WrOnlyFlag | CreateFlag, nil},
		{WrOnlyFlag | ExclFlag, nil},
		{WrOnlyFlag, nil},
		{WrOnlyFlag | TruncFlag, nil},
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
