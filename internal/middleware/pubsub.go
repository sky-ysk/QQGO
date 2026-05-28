package middleware

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/redis/go-redis/v9"
	"github.com/qqgo/server/internal/model"
)

type PubSubMessage struct {
	Source  string          `json:"source"`
	Message json.RawMessage `json:"msg"`
}

type MessageHandler func(qq int64, msg *model.Message)

type PubSubRouter struct {
	rdb        *redis.Client
	instanceID string
	handler    MessageHandler
	pubsub     *redis.PubSub
	ctx        context.Context
	cancel     context.CancelFunc
}

func NewPubSubRouter(rdb *redis.Client, instanceID string, handler MessageHandler) *PubSubRouter {
	ctx, cancel := context.WithCancel(context.Background())
	return &PubSubRouter{
		rdb:        rdb,
		instanceID: instanceID,
		handler:    handler,
		ctx:        ctx,
		cancel:     cancel,
	}
}

func (p *PubSubRouter) Start() {
	if p == nil || p.rdb == nil {
		return
	}
	p.pubsub = p.rdb.Subscribe(p.ctx)
	go p.listenLoop()
}

func (p *PubSubRouter) Stop() {
	if p == nil {
		return
	}
	p.cancel()
	if p.pubsub != nil {
		p.pubsub.Close()
	}
}

func (p *PubSubRouter) Subscribe(channels ...string) {
	if p == nil || p.pubsub == nil || len(channels) == 0 {
		return
	}
	p.pubsub.Subscribe(p.ctx, channels...)
}

func (p *PubSubRouter) Unsubscribe(channels ...string) {
	if p == nil || p.pubsub == nil || len(channels) == 0 {
		return
	}
	p.pubsub.Unsubscribe(p.ctx, channels...)
}

func (p *PubSubRouter) PublishToUser(qq int64, msg *model.Message) {
	if p == nil || p.rdb == nil {
		return
	}
	data, _ := json.Marshal(msg)
	payload := PubSubMessage{Source: p.instanceID, Message: data}
	payloadBytes, _ := json.Marshal(payload)
	channel := fmt.Sprintf("ch:qq:%d", qq)
	p.rdb.Publish(p.ctx, channel, payloadBytes)
}

func (p *PubSubRouter) PublishToGroup(groupID string, msg *model.Message) {
	if p == nil || p.rdb == nil {
		return
	}
	data, _ := json.Marshal(msg)
	payload := PubSubMessage{Source: p.instanceID, Message: data}
	payloadBytes, _ := json.Marshal(payload)
	channel := fmt.Sprintf("ch:group:%s", groupID)
	p.rdb.Publish(p.ctx, channel, payloadBytes)
}

func (p *PubSubRouter) listenLoop() {
	ch := p.pubsub.Channel()
	for {
		select {
		case <-p.ctx.Done():
			return
		case redisMsg, ok := <-ch:
			if !ok {
				return
			}
			p.handleMessage(redisMsg)
		}
	}
}

func (p *PubSubRouter) handleMessage(redisMsg *redis.Message) {
	var pubsubMsg PubSubMessage
	if err := json.Unmarshal([]byte(redisMsg.Payload), &pubsubMsg); err != nil {
		return
	}
	if pubsubMsg.Source == p.instanceID {
		return
	}
	var msg model.Message
	if err := json.Unmarshal(pubsubMsg.Message, &msg); err != nil {
		return
	}
	if msg.ToQQ != 0 && p.handler != nil {
		p.handler(msg.ToQQ, &msg)
	}
	if msg.GroupID != "" && msg.ToQQ == 0 && p.handler != nil {
		p.handler(0, &msg)
	}
}
