package server

import (
	"testing"
	"time"
)

func TestUploadAndVoiceRateLimits(t *testing.T) {
	now := time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)
	l := newRateLimiter(3, 10*time.Minute)
	l.now = func() time.Time { return now }
	for i := range 3 {
		if ok, _ := l.allow(1); !ok {
			t.Fatalf("request %d was refused within the limit", i)
		}
	}
	if ok, wait := l.allow(1); ok || wait <= 0 || wait > 10*time.Minute {
		t.Fatalf("the fourth request must wait, got %v %s", ok, wait)
	}
	if ok, _ := l.allow(2); !ok {
		t.Fatal("one owner's limit must not affect another")
	}
	now = now.Add(11 * time.Minute)
	if ok, _ := l.allow(1); !ok {
		t.Fatal("the window must slide")
	}
	if uploadLimit.limit <= 0 || voiceLimit.limit <= 0 {
		t.Fatal("uploads and voice must be limited")
	}
}
