package middleware

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
)

type OnlineTracker struct {
	rdb        *redis.Client
	instanceID string
}

func NewOnlineTracker(rdb *redis.Client, instanceID string) *OnlineTracker {
	return &OnlineTracker{rdb: rdb, instanceID: instanceID}
}

func (o *OnlineTracker) SetOnline(qq int64) {
	if o == nil || o.rdb == nil {
		return
	}
	key := fmt.Sprintf("online:%d", qq)
	o.rdb.Set(context.Background(), key, o.instanceID, 60e9)
}

func (o *OnlineTracker) SetOffline(qq int64) {
	if o == nil || o.rdb == nil {
		return
	}
	key := fmt.Sprintf("online:%d", qq)
	val, err := o.rdb.Get(context.Background(), key).Result()
	if err != nil || val != o.instanceID {
		return
	}
	o.rdb.Del(context.Background(), key)
}

func (o *OnlineTracker) RefreshOnline(qq int64) {
	if o == nil || o.rdb == nil {
		return
	}
	key := fmt.Sprintf("online:%d", qq)
	o.rdb.Expire(context.Background(), key, 60e9)
}

func (o *OnlineTracker) GetInstance(qq int64) (string, bool) {
	if o == nil || o.rdb == nil {
		return "", false
	}
	key := fmt.Sprintf("online:%d", qq)
	val, err := o.rdb.Get(context.Background(), key).Result()
	if err != nil {
		return "", false
	}
	return val, true
}

func (o *OnlineTracker) CountOnline() int {
	if o == nil || o.rdb == nil {
		return 0
	}
	keys, _ := o.rdb.Keys(context.Background(), "online:*").Result()
	return len(keys)
}
