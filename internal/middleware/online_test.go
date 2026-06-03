package middleware

import (
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func setupMiniRedis(t *testing.T) (*miniredis.Miniredis, *redis.Client) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	return mr, rdb
}

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

func TestOnlineTrackerSetAndGet(t *testing.T) {
	_, rdb := setupMiniRedis(t)
	tracker := NewOnlineTracker(rdb, "instance-1")

	instance, ok := tracker.GetInstance(10001)
	if ok {
		t.Fatal("user should not be online yet")
	}
	if instance != "" {
		t.Fatal("instance should be empty")
	}

	tracker.SetOnline(10001)

	instance, ok = tracker.GetInstance(10001)
	if !ok {
		t.Fatal("user should be online")
	}
	if instance != "instance-1" {
		t.Fatalf("expected instance-1, got %s", instance)
	}
}

func TestOnlineTrackerSetOffline(t *testing.T) {
	_, rdb := setupMiniRedis(t)
	tracker := NewOnlineTracker(rdb, "instance-1")

	tracker.SetOnline(10001)
	tracker.SetOffline(10001)

	_, ok := tracker.GetInstance(10001)
	if ok {
		t.Fatal("user should be offline after SetOffline")
	}
}

func TestOnlineTrackerSetOfflineOtherInstance(t *testing.T) {
	_, rdb := setupMiniRedis(t)
	tracker1 := NewOnlineTracker(rdb, "instance-1")
	tracker2 := NewOnlineTracker(rdb, "instance-2")

	tracker1.SetOnline(10001)

	tracker2.SetOffline(10001)

	_, ok := tracker1.GetInstance(10001)
	if !ok {
		t.Fatal("instance-2 should not be able to set offline a user owned by instance-1")
	}
}

func TestOnlineTrackerRefresh(t *testing.T) {
	mr, rdb := setupMiniRedis(t)
	tracker := NewOnlineTracker(rdb, "instance-1")

	tracker.SetOnline(10001)

	tracker.RefreshOnline(10001)

	ttl := mr.TTL("online:10001")
	if ttl <= 0 {
		t.Fatal("TTL should be positive after refresh")
	}
}

func TestOnlineTrackerCountOnline(t *testing.T) {
	_, rdb := setupMiniRedis(t)
	tracker := NewOnlineTracker(rdb, "instance-1")

	if tracker.CountOnline() != 0 {
		t.Fatal("should have 0 online users initially")
	}

	tracker.SetOnline(10001)
	tracker.SetOnline(10002)
	tracker.SetOnline(10003)

	if tracker.CountOnline() != 3 {
		t.Fatalf("expected 3 online users, got %d", tracker.CountOnline())
	}

	tracker.SetOffline(10002)

	if tracker.CountOnline() != 2 {
		t.Fatalf("expected 2 online users after one goes offline, got %d", tracker.CountOnline())
	}
}

func TestOnlineTrackerNilRedis(t *testing.T) {
	tracker := &OnlineTracker{rdb: nil, instanceID: "instance-1"}
	tracker.SetOnline(10001)
	tracker.SetOffline(10001)
	tracker.RefreshOnline(10001)
	_, ok := tracker.GetInstance(10001)
	if ok {
		t.Fatal("nil redis should return false")
	}
	if tracker.CountOnline() != 0 {
		t.Fatal("nil redis should return 0")
	}
}
