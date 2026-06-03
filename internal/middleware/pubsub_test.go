package middleware

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	pb "github.com/qqgo/server/internal/protocol"
	"google.golang.org/protobuf/proto"
)

func setupMiniRedisPubSub(t *testing.T) (*miniredis.Miniredis, *redis.Client) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	return mr, rdb
}

func TestPubSubRouterNil(t *testing.T) {
	var router *PubSubRouter
	router.Start()
	router.Stop()
	router.Subscribe("ch:qq:10001")
	router.Unsubscribe("ch:qq:10001")
	router.PublishToUser(10001, nil)
	router.PublishToGroup("G1", nil)
}

func TestHandleMessageToUser(t *testing.T) {
	var receivedQQ int64
	var receivedMsg *pb.WireMessage

	handler := func(qq int64, msg *pb.WireMessage) {
		receivedQQ = qq
		receivedMsg = msg
	}

	router := &PubSubRouter{
		instanceID: "instance-1",
		handler:    handler,
	}

	wireMsg := &pb.WireMessage{
		Id:     1,
		FromQq: 10001,
		ToQq:   10002,
	}
	data, _ := proto.Marshal(wireMsg)
	payload := PubSubMessage{Source: "instance-2", Message: data}
	payloadBytes, _ := json.Marshal(payload)

	redisMsg := &redis.Message{Payload: string(payloadBytes)}
	router.handleMessage(redisMsg)

	if receivedQQ != 10002 {
		t.Fatalf("expected qq 10002, got %d", receivedQQ)
	}
	if receivedMsg == nil {
		t.Fatal("expected message to be received")
	}
	if receivedMsg.FromQq != 10001 {
		t.Fatalf("expected from_qq 10001, got %d", receivedMsg.FromQq)
	}
}

func TestHandleMessageToGroup(t *testing.T) {
	var receivedQQ int64
	var receivedMsg *pb.WireMessage

	handler := func(qq int64, msg *pb.WireMessage) {
		receivedQQ = qq
		receivedMsg = msg
	}

	router := &PubSubRouter{
		instanceID: "instance-1",
		handler:    handler,
	}

	wireMsg := &pb.WireMessage{
		Id:      1,
		FromQq:  10001,
		GroupId: "G123",
	}
	data, _ := proto.Marshal(wireMsg)
	payload := PubSubMessage{Source: "instance-2", Message: data}
	payloadBytes, _ := json.Marshal(payload)

	redisMsg := &redis.Message{Payload: string(payloadBytes)}
	router.handleMessage(redisMsg)

	if receivedQQ != 0 {
		t.Fatalf("expected qq 0 for group message, got %d", receivedQQ)
	}
	if receivedMsg == nil {
		t.Fatal("expected group message to be received")
	}
	if receivedMsg.GroupId != "G123" {
		t.Fatalf("expected group_id G123, got %s", receivedMsg.GroupId)
	}
}

func TestHandleMessageSkipsSelf(t *testing.T) {
	called := false
	handler := func(qq int64, msg *pb.WireMessage) {
		called = true
	}

	router := &PubSubRouter{
		instanceID: "instance-1",
		handler:    handler,
	}

	wireMsg := &pb.WireMessage{Id: 1, FromQq: 10001, ToQq: 10002}
	data, _ := proto.Marshal(wireMsg)
	payload := PubSubMessage{Source: "instance-1", Message: data}
	payloadBytes, _ := json.Marshal(payload)

	redisMsg := &redis.Message{Payload: string(payloadBytes)}
	router.handleMessage(redisMsg)

	if called {
		t.Fatal("handler should not be called for self-originated messages")
	}
}

func TestHandleMessageInvalidJSON(t *testing.T) {
	called := false
	handler := func(qq int64, msg *pb.WireMessage) {
		called = true
	}

	router := &PubSubRouter{
		instanceID: "instance-1",
		handler:    handler,
	}

	redisMsg := &redis.Message{Payload: "not-valid-json"}
	router.handleMessage(redisMsg)

	if called {
		t.Fatal("handler should not be called for invalid JSON")
	}
}

func TestHandleMessageInvalidProtobuf(t *testing.T) {
	called := false
	handler := func(qq int64, msg *pb.WireMessage) {
		called = true
	}

	router := &PubSubRouter{
		instanceID: "instance-1",
		handler:    handler,
	}

	payload := PubSubMessage{Source: "instance-2", Message: []byte("not-protobuf")}
	payloadBytes, _ := json.Marshal(payload)

	redisMsg := &redis.Message{Payload: string(payloadBytes)}
	router.handleMessage(redisMsg)

	if called {
		t.Fatal("handler should not be called for invalid protobuf")
	}
}

func TestHandleMessageNilHandler(t *testing.T) {
	router := &PubSubRouter{
		instanceID: "instance-1",
		handler:    nil,
	}

	wireMsg := &pb.WireMessage{Id: 1, FromQq: 10001, ToQq: 10002}
	data, _ := proto.Marshal(wireMsg)
	payload := PubSubMessage{Source: "instance-2", Message: data}
	payloadBytes, _ := json.Marshal(payload)

	redisMsg := &redis.Message{Payload: string(payloadBytes)}
	router.handleMessage(redisMsg)
}

func TestSubscribeEmptyChannels(t *testing.T) {
	router := &PubSubRouter{
		instanceID: "instance-1",
	}
	router.Subscribe()
	router.Unsubscribe()
}

func TestPubSubRouterStartStop(t *testing.T) {
	_, rdb := setupMiniRedisPubSub(t)

	handler := func(qq int64, msg *pb.WireMessage) {}
	router := NewPubSubRouter(rdb, "instance-1", handler)

	router.Start()
	router.Subscribe("ch:qq:10001", "ch:group:G1")
	router.Stop()
}

func TestPubSubRouterPublishToUser(t *testing.T) {
	_, rdb := setupMiniRedisPubSub(t)

	var receivedQQ int64
	var receivedMsg *pb.WireMessage
	done := make(chan struct{})

	handler := func(qq int64, msg *pb.WireMessage) {
		receivedQQ = qq
		receivedMsg = msg
		close(done)
	}

	router1 := NewPubSubRouter(rdb, "instance-1", handler)
	router1.Start()
	router1.Subscribe("ch:qq:10002")

	router2 := NewPubSubRouter(rdb, "instance-2", nil)

	wireMsg := &pb.WireMessage{Id: 42, FromQq: 10001, ToQq: 10002}
	router2.PublishToUser(10002, wireMsg)

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for pubsub message")
	}

	if receivedQQ != 10002 {
		t.Fatalf("expected qq 10002, got %d", receivedQQ)
	}
	if receivedMsg == nil {
		t.Fatal("expected message")
	}
	if receivedMsg.Id != 42 {
		t.Fatalf("expected message id 42, got %d", receivedMsg.Id)
	}

	router1.Stop()
}

func TestPubSubRouterPublishToGroup(t *testing.T) {
	_, rdb := setupMiniRedisPubSub(t)

	var receivedQQ int64
	var receivedMsg *pb.WireMessage
	done := make(chan struct{})

	handler := func(qq int64, msg *pb.WireMessage) {
		receivedQQ = qq
		receivedMsg = msg
		close(done)
	}

	router1 := NewPubSubRouter(rdb, "instance-1", handler)
	router1.Start()
	router1.Subscribe("ch:group:G123")

	router2 := NewPubSubRouter(rdb, "instance-2", nil)

	wireMsg := &pb.WireMessage{Id: 99, FromQq: 10001, GroupId: "G123"}
	router2.PublishToGroup("G123", wireMsg)

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for group pubsub message")
	}

	if receivedQQ != 0 {
		t.Fatalf("expected qq 0 for group message, got %d", receivedQQ)
	}
	if receivedMsg == nil {
		t.Fatal("expected message")
	}
	if receivedMsg.GroupId != "G123" {
		t.Fatalf("expected group_id G123, got %s", receivedMsg.GroupId)
	}

	router1.Stop()
}

func TestPubSubRouterSkipsSelfPublished(t *testing.T) {
	_, rdb := setupMiniRedisPubSub(t)

	called := false
	handler := func(qq int64, msg *pb.WireMessage) {
		called = true
	}

	router := NewPubSubRouter(rdb, "instance-1", handler)
	router.Start()
	router.Subscribe("ch:qq:10002")

	wireMsg := &pb.WireMessage{Id: 1, FromQq: 10001, ToQq: 10002}
	router.PublishToUser(10002, wireMsg)

	time.Sleep(200 * time.Millisecond)

	if called {
		t.Fatal("should not receive self-published messages")
	}

	router.Stop()
}

func TestPubSubRouterUnsubscribe(t *testing.T) {
	_, rdb := setupMiniRedisPubSub(t)

	handler := func(qq int64, msg *pb.WireMessage) {}
	router := NewPubSubRouter(rdb, "instance-1", handler)
	router.Start()
	router.Subscribe("ch:qq:10001")
	router.Unsubscribe("ch:qq:10001")
	router.Stop()
}

func TestPubSubRouterNilRedis(t *testing.T) {
	router := NewPubSubRouter(nil, "instance-1", nil)
	router.Start()
	router.PublishToUser(10001, nil)
	router.PublishToGroup("G1", nil)
	router.Stop()
}
