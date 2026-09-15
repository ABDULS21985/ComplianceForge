package repository

import (
	"errors"
	"math"
	"testing"
)

type identityPasskeyCounterRow struct {
	count int64
	err   error
}

func (row identityPasskeyCounterRow) Scan(destinations ...any) error {
	if row.err != nil {
		return row.err
	}
	*destinations[7].(*int64) = row.count
	return nil
}

func TestScanIdentityPasskeyRejectsCounterNarrowingOverflow(t *testing.T) {
	for _, count := range []int64{math.MinInt64, -1, int64(math.MaxUint32) + 1, math.MaxInt64} {
		item, err := scanIdentityPasskey(identityPasskeyCounterRow{count: count})
		if err == nil || item != nil {
			t.Fatalf("invalid counter %d returned a usable passkey", count)
		}
	}
}

func TestScanIdentityPasskeyPreservesWebAuthnCounterBounds(t *testing.T) {
	for _, count := range []int64{0, 1, int64(math.MaxUint32) - 1, math.MaxUint32} {
		item, err := scanIdentityPasskey(identityPasskeyCounterRow{count: count})
		if err != nil || item == nil || int64(item.SignCount) != count {
			t.Fatalf("valid counter %d was not preserved: item=%#v err=%v", count, item, err)
		}
	}
}

func TestScanIdentityPasskeyPropagatesScanFailureWithoutPartialCredential(t *testing.T) {
	want := errors.New("scan failed")
	item, err := scanIdentityPasskey(identityPasskeyCounterRow{err: want})
	if !errors.Is(err, want) || item != nil {
		t.Fatalf("scan failure returned a usable credential: item=%#v err=%v", item, err)
	}
}
