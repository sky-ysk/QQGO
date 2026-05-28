package middleware

import (
	"testing"
)

func TestOnlineTrackerNil(t *testing.T) {
	var tracker *OnlineTracker
	tracker.SetOnline(10001)
	tracker.SetOffline(10001)
	tracker.RefreshOnline(10001)
	instance, ok := tracker.GetInstance(10001)
	if ok {
		t.Fatal("nil tracker should return false for GetInstance")
	}
	if instance != "" {
		t.Fatal("nil tracker should return empty instance")
	}
	count := tracker.CountOnline()
	if count != 0 {
		t.Fatal("nil tracker should return 0 for CountOnline")
	}
}
