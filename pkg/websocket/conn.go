package websocket

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = 30 * time.Second
	maxMessageSize = 4096
)

type Conn struct {
	WS       *websocket.Conn
	QQ       int64
	Platform string
	mu       sync.Mutex
	Send     chan []byte
	pingCh   chan struct{}
	done     chan struct{}
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
			if err := c.WS.WriteMessage(websocket.TextMessage, msg); err != nil {
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

func (c *Conn) WriteJSON(v interface{}) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	data, err := json.Marshal(v)
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
	c.mu.Lock()
	defer c.mu.Unlock()

	select {
	case <-c.done:
		return
	default:
	}
	close(c.Send)
}

func (c *Conn) Done() <-chan struct{} {
	return c.done
}
