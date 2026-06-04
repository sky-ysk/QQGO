package websocket

import (
	"fmt"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"
)

const (
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = 30 * time.Second
	maxMessageSize = 10 * 1024 * 1024
)

type Conn struct {
	WS       *websocket.Conn
	QQ       int64
	Platform string
	mu       sync.Mutex
	Send     chan []byte
	pingCh   chan struct{}
	done     chan struct{}
	closeOnce sync.Once
}

func NewConn(ws *websocket.Conn) *Conn {
	return &Conn{
		WS:     ws,
		Send:   make(chan []byte, 256),
		pingCh: make(chan struct{}, 1),
		done:   make(chan struct{}),
	}
}

func (c *Conn) ReadLoop(handler func(msgType int, data []byte)) {
	c.WS.SetReadLimit(maxMessageSize)
	c.WS.SetReadDeadline(time.Now().Add(pongWait))
	c.WS.SetPongHandler(func(string) error {
		c.WS.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		msgType, data, err := c.WS.ReadMessage()
		if err != nil {
			break
		}
		handler(msgType, data)
	}

	close(c.done)
}

func (c *Conn) WriteLoop() {
	ticker := time.NewTicker(pingPeriod)
	defer ticker.Stop()
	defer c.WS.Close()

	for {
		select {
		case msg, ok := <-c.Send:
			c.WS.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				c.WS.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.WS.WriteMessage(websocket.BinaryMessage, msg); err != nil {
				return
			}
		case <-ticker.C:
			c.WS.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.WS.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		case <-c.done:
			return
		case <-c.pingCh:
			c.WS.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.WS.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func (c *Conn) WriteProto(msg proto.Message) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("connection closed")
		}
	}()

	data, err := proto.Marshal(msg)
	if err != nil {
		return err
	}

	select {
	case c.Send <- data:
	default:
		go c.Close()
	}
	return nil
}

func (c *Conn) Close() {
	c.closeOnce.Do(func() {
		close(c.Send)
	})
}

func (c *Conn) Done() <-chan struct{} {
	return c.done
}
