package erroradvisor

import (
	"context"
	"testing"

	relayerrors "relay/internal/errors"
)

func TestSuggestForTimeout(t *testing.T) {
	msg := Suggest(context.DeadlineExceeded)
	if msg == "" {
		t.Fatal("expected timeout suggestion")
	}
}

func TestSuggestForCacheOverflow(t *testing.T) {
	err := relayerrors.New(relayerrors.CategoryCache, "entry_overflow", "cache entry too large")
	msg := Suggest(err)
	if msg != "Increase cache size limits or adjust eviction policy settings." {
		t.Fatalf("suggestion = %q", msg)
	}
}

func TestSuggestForUnknownError(t *testing.T) {
	msg := Suggest(nil)
	if msg != "" {
		t.Fatalf("suggestion = %q, want empty", msg)
	}
}
