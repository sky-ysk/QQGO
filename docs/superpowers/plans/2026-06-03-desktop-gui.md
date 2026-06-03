# QQGO 桌面端 GUI 实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 使用 Wails v2 构建 QQGO 桌面客户端，实现登录/注册、会话列表、私聊/群聊消息收发。

**Architecture:** Wails v2 框架，Go 后端管理 WebSocket 连接和 Protobuf 消息收发，前端使用原生 HTML/CSS/JS 渲染 QQ 风格三栏界面。Go 层通过 Wails Bindings 暴露方法给前端，通过 Wails Events 推送消息到前端。

**Tech Stack:** Wails v2, gorilla/websocket, google.golang.org/protobuf, vanilla JavaScript

---

## 文件结构

```
cmd/desktop/
├── main.go                 # Wails 入口
├── app.go                  # App struct + WebSocket 管理 + Wails 绑定
├── app_test.go             # Go 层单元测试
├── frontend/
│   ├── index.html          # 单页应用
│   ├── src/
│   │   ├── main.js         # 应用初始化 + 路由
│   │   ├── api.js          # Wails bindings 封装
│   │   ├── store.js        # 状态管理
│   │   ├── views/
│   │   │   ├── login.js    # 登录/注册视图
│   │   │   └── chat.js     # 主窗口视图
│   │   └── components/
│   │       ├── sidebar.js  # 左侧导航
│   │       ├── sessions.js # 会话列表
│   │       └── messages.js # 聊天区域
│   └── styles/
│       └── main.css        # QQ 蓝主题样式
```

---

## Task 1: 初始化 Wails 项目

**Files:**
- Create: `cmd/desktop/main.go`
- Create: `cmd/desktop/app.go`

- [ ] **Step 1: 安装 Wails CLI**

```bash
go install github.com/wailsapp/wails/v2/cmd/wails@latest
wails version
```

Expected: 输出 Wails v2.x.x 版本信息

- [ ] **Step 2: 创建 Wails 项目结构**

```bash
cd /Users/yangshikang.6/Desktop/Code/Go/QQGO/cmd/desktop
wails init -n qqgo-desktop -t vanilla
```

Expected: 生成 `main.go`, `app.go`, `frontend/` 目录

- [ ] **Step 3: 修改 main.go 配置窗口**

```go
package main

import (
	"embed"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend
var assets embed.FS

func main() {
	app := NewApp()

	err := wails.Run(&options.App{
		Title:  "QQGO",
		Width:  960,
		Height: 640,
		MinWidth:  720,
		MinHeight: 480,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		OnStartup: app.startup,
		Bind: []interface{}{
			app,
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}
```

- [ ] **Step 4: 创建基础 app.go**

```go
package main

import (
	"context"
)

type App struct {
	ctx context.Context
}

func NewApp() *App {
	return &App{}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}
```

- [ ] **Step 5: 验证项目可运行**

```bash
cd /Users/yangshikang.6/Desktop/Code/Go/QQGO/cmd/desktop
wails dev
```

Expected: 打开窗口显示 Wails 默认页面

- [ ] **Step 6: Commit**

```bash
git add cmd/desktop/
git commit -m "feat(desktop): initialize Wails v2 project structure"
```

---

## Task 2: Go 后端 - WebSocket 客户端

**Files:**
- Modify: `cmd/desktop/app.go`

- [ ] **Step 1: 添加 WebSocket 连接管理**

在 `app.go` 中添加 WebSocket 连接、发送、接收逻辑：

```go
package main

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/qqgo/server/internal/protocol"
	"github.com/wailsapp/wails/v2/pkg/runtime"
	"google.golang.org/protobuf/proto"
)

type App struct {
	ctx  context.Context
	conn *websocket.Conn
	qq   int64
	mu   sync.Mutex
	seq  int64
}

func NewApp() *App {
	return &App{}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

func (a *App) shutdown(ctx context.Context) {
	a.Disconnect()
}

func (a *App) Connect(addr string) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.conn != nil {
		a.conn.Close()
	}

	conn, _, err := websocket.DefaultDialer.Dial(addr, nil)
	if err != nil {
		return err
	}

	a.conn = conn
	go a.readLoop()
	go a.heartbeatLoop()

	return nil
}

func (a *App) Disconnect() {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.conn != nil {
		a.conn.Close()
		a.conn = nil
	}
}

func (a *App) sendWire(wire *protocol.WireMessage) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.conn == nil {
		return fmt.Errorf("not connected")
	}

	data, err := proto.Marshal(wire)
	if err != nil {
		return err
	}

	return a.conn.WriteMessage(websocket.BinaryMessage, data)
}

func (a *App) nextSeq() int64 {
	a.seq++
	return a.seq
}

func (a *App) readLoop() {
	defer func() {
		runtime.EventsEmit(a.ctx, "connection-lost")
	}()

	for {
		_, data, err := a.conn.ReadMessage()
		if err != nil {
			log.Printf("[ws] read error: %v", err)
			return
		}

		var wire protocol.WireMessage
		if err := proto.Unmarshal(data, &wire); err != nil {
			log.Printf("[ws] unmarshal error: %v", err)
			continue
		}

		a.handleMessage(&wire)
	}
}

func (a *App) heartbeatLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		a.mu.Lock()
		if a.conn == nil {
			a.mu.Unlock()
			return
		}
		a.mu.Unlock()

		a.sendWire(&protocol.WireMessage{
			Payload: &protocol.WireMessage_Heartbeat{
				Heartbeat: &protocol.Heartbeat{Content: "ping"},
			},
		})
	}
}

func (a *App) handleMessage(wire *protocol.WireMessage) {
	switch p := wire.Payload.(type) {
	case *protocol.WireMessage_LoginResponse:
		if p.LoginResponse.Code == 0 {
			a.qq = p.LoginResponse.QqNumber
			runtime.EventsEmit(a.ctx, "login-success", map[string]interface{}{
				"qq":          p.LoginResponse.QqNumber,
				"nickname":    p.LoginResponse.Nickname,
				"accessToken": p.LoginResponse.AccessToken,
			})
		} else {
			runtime.EventsEmit(a.ctx, "login-failed", map[string]interface{}{
				"message": p.LoginResponse.Message,
			})
		}

	case *protocol.WireMessage_RegisterResponse:
		if p.RegisterResponse.Code == 0 {
			runtime.EventsEmit(a.ctx, "register-success", map[string]interface{}{
				"qq": p.RegisterResponse.QqNumber,
			})
		} else {
			runtime.EventsEmit(a.ctx, "register-failed", map[string]interface{}{
				"message": p.RegisterResponse.Message,
			})
		}

	case *protocol.WireMessage_SessionListResponse:
		sessions := make([]map[string]interface{}, 0, len(p.SessionListResponse.Sessions))
		for _, s := range p.SessionListResponse.Sessions {
			sessions = append(sessions, map[string]interface{}{
				"type":        s.Type,
				"targetQQ":    s.TargetQq,
				"groupID":     s.GroupId,
				"nickname":    s.Nickname,
				"lastMessage": s.LastMessage,
				"lastTime":    s.LastTime,
				"online":      s.Online,
			})
		}
		runtime.EventsEmit(a.ctx, "sessions-updated", sessions)

	case *protocol.WireMessage_FriendListResponse:
		friends := make([]map[string]interface{}, 0, len(p.FriendListResponse.Friends))
		for _, f := range p.FriendListResponse.Friends {
			friends = append(friends, map[string]interface{}{
				"qqNumber":  f.QqNumber,
				"nickname":  f.Nickname,
				"groupName": f.GroupName,
				"online":    f.Online,
			})
		}
		runtime.EventsEmit(a.ctx, "friends-loaded", friends)

	case *protocol.WireMessage_HistoryResponse:
		msgs := make([]map[string]interface{}, 0, len(p.HistoryResponse.Messages))
		for _, m := range p.HistoryResponse.Messages {
			msgs = append(msgs, map[string]interface{}{
				"id":        m.Id,
				"fromQQ":    m.FromQq,
				"toQQ":      m.ToQq,
				"content":   m.Content,
				"createdAt": m.CreatedAt,
			})
		}
		runtime.EventsEmit(a.ctx, "history-loaded", msgs)

	case *protocol.WireMessage_TextMessage:
		runtime.EventsEmit(a.ctx, "message-received", map[string]interface{}{
			"id":        wire.Id,
			"fromQQ":    wire.FromQq,
			"toQQ":      wire.ToQq,
			"groupID":   wire.GroupId,
			"content":   p.TextMessage.Content,
			"createdAt": wire.CreatedAt,
		})

	case *protocol.WireMessage_ServerAck:
		runtime.EventsEmit(a.ctx, "message-ack", map[string]interface{}{
			"id":        wire.Id,
			"clientSeq": wire.ClientSeq,
		})
	}
}
```

- [ ] **Step 2: 验证编译通过**

```bash
cd /Users/yangshikang.6/Desktop/Code/Go/QQGO/cmd/desktop
go build
```

Expected: 编译成功无错误

- [ ] **Step 3: Commit**

```bash
git add cmd/desktop/app.go
git commit -m "feat(desktop): add WebSocket client connection management"
```

---

## Task 3: Go 后端 - Wails 绑定方法

**Files:**
- Modify: `cmd/desktop/app.go`

- [ ] **Step 1: 添加登录/注册方法**

在 `app.go` 中添加：

```go
func (a *App) Login(qq int64, password string) error {
	return a.sendWire(&protocol.WireMessage{
		ClientSeq: a.nextSeq(),
		Payload: &protocol.WireMessage_LoginRequest{
			LoginRequest: &protocol.LoginRequest{
				Qq:       qq,
				Password: password,
				Platform: "desktop",
			},
		},
	})
}

func (a *App) Register(nickname, password string) error {
	return a.sendWire(&protocol.WireMessage{
		ClientSeq: a.nextSeq(),
		Payload: &protocol.WireMessage_RegisterRequest{
			RegisterRequest: &protocol.RegisterRequest{
				Nickname: nickname,
				Password: password,
			},
		},
	})
}
```

- [ ] **Step 2: 添加消息发送方法**

```go
func (a *App) SendMessage(toQQ int64, content string) error {
	return a.sendWire(&protocol.WireMessage{
		ClientSeq: a.nextSeq(),
		ToQq:      toQQ,
		Payload: &protocol.WireMessage_TextMessage{
			TextMessage: &protocol.TextMessage{
				Content: content,
			},
		},
	})
}

func (a *App) SendGroupMessage(groupID, content string) error {
	return a.sendWire(&protocol.WireMessage{
		ClientSeq: a.nextSeq(),
		GroupId:   groupID,
		Payload: &protocol.WireMessage_TextMessage{
			TextMessage: &protocol.TextMessage{
				Content: content,
			},
		},
	})
}
```

- [ ] **Step 3: 添加数据请求方法**

```go
func (a *App) GetSessions() error {
	return a.sendWire(&protocol.WireMessage{
		ClientSeq: a.nextSeq(),
		Payload: &protocol.WireMessage_SessionListRequest{
			SessionListRequest: &protocol.SessionListRequest{},
		},
	})
}

func (a *App) GetFriendList() error {
	return a.sendWire(&protocol.WireMessage{
		ClientSeq: a.nextSeq(),
		Payload: &protocol.WireMessage_FriendListRequest{
			FriendListRequest: &protocol.FriendListRequest{},
		},
	})
}

func (a *App) GetHistory(targetQQ int64, offset, limit int32) error {
	return a.sendWire(&protocol.WireMessage{
		ClientSeq: a.nextSeq(),
		Payload: &protocol.WireMessage_HistoryRequest{
			HistoryRequest: &protocol.HistoryRequest{
				TargetQq: targetQQ,
				Offset:   offset,
				Limit:    limit,
			},
		},
	})
}
```

- [ ] **Step 4: 验证编译通过**

```bash
cd /Users/yangshikang.6/Desktop/Code/Go/QQGO/cmd/desktop
go build
```

Expected: 编译成功

- [ ] **Step 5: Commit**

```bash
git add cmd/desktop/app.go
git commit -m "feat(desktop): add Wails binding methods for login, messages, sessions"
```

---

## Task 4: 前端 - QQ 蓝主题样式

**Files:**
- Create: `cmd/desktop/frontend/styles/main.css`

- [ ] **Step 1: 创建 CSS 主题文件**

```css
:root {
  --primary: #12B7F5;
  --sidebar-bg: #2B2F3A;
  --sidebar-icon: #8E94A5;
  --session-bg: #FFFFFF;
  --session-active: #E6F7FF;
  --chat-bg: #F5F5F5;
  --bubble-other: #FFFFFF;
  --bubble-self: #12B7F5;
  --text-primary: #333333;
  --text-secondary: #999999;
  --border: #E5E5E5;
}

* {
  margin: 0;
  padding: 0;
  box-sizing: border-box;
}

body {
  font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
  font-size: 14px;
  color: var(--text-primary);
  overflow: hidden;
}

#app {
  width: 100vw;
  height: 100vh;
  display: flex;
}

/* Login Page */
.login-page {
  width: 100%;
  height: 100%;
  display: flex;
  align-items: center;
  justify-content: center;
  background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
}

.login-card {
  background: white;
  border-radius: 12px;
  padding: 40px;
  width: 360px;
  box-shadow: 0 20px 60px rgba(0,0,0,0.3);
}

.login-card h1 {
  text-align: center;
  color: var(--primary);
  margin-bottom: 30px;
  font-size: 28px;
}

.login-card .tabs {
  display: flex;
  margin-bottom: 20px;
  border-bottom: 2px solid var(--border);
}

.login-card .tab {
  flex: 1;
  padding: 10px;
  text-align: center;
  cursor: pointer;
  color: var(--text-secondary);
  border-bottom: 2px solid transparent;
  margin-bottom: -2px;
}

.login-card .tab.active {
  color: var(--primary);
  border-bottom-color: var(--primary);
}

.login-card input {
  width: 100%;
  padding: 12px 16px;
  margin-bottom: 16px;
  border: 1px solid var(--border);
  border-radius: 8px;
  font-size: 14px;
}

.login-card input:focus {
  outline: none;
  border-color: var(--primary);
}

.login-card button {
  width: 100%;
  padding: 12px;
  background: var(--primary);
  color: white;
  border: none;
  border-radius: 8px;
  font-size: 16px;
  cursor: pointer;
}

.login-card button:hover {
  background: #0fa3dc;
}

.login-card .server-addr {
  margin-top: 20px;
  padding-top: 20px;
  border-top: 1px solid var(--border);
}

.login-card .server-addr label {
  display: block;
  font-size: 12px;
  color: var(--text-secondary);
  margin-bottom: 8px;
}

/* Main Window */
.main-window {
  width: 100%;
  height: 100%;
  display: flex;
}

/* Sidebar */
.sidebar {
  width: 54px;
  background: var(--sidebar-bg);
  display: flex;
  flex-direction: column;
  align-items: center;
  padding-top: 16px;
  gap: 20px;
}

.sidebar .avatar {
  width: 36px;
  height: 36px;
  border-radius: 50%;
  background: var(--primary);
  display: flex;
  align-items: center;
  justify-content: center;
  color: white;
  font-size: 14px;
  font-weight: bold;
}

.sidebar .nav-item {
  width: 28px;
  height: 28px;
  border-radius: 6px;
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--sidebar-icon);
  cursor: pointer;
  font-size: 16px;
}

.sidebar .nav-item:hover {
  background: rgba(255,255,255,0.1);
}

.sidebar .nav-item.active {
  color: var(--primary);
  background: rgba(18,183,245,0.1);
}

.sidebar .spacer {
  flex: 1;
}

/* Session List */
.session-list {
  width: 240px;
  background: var(--session-bg);
  border-right: 1px solid var(--border);
  display: flex;
  flex-direction: column;
}

.session-list .search-bar {
  padding: 12px 14px;
  border-bottom: 1px solid var(--border);
}

.session-list .search-bar input {
  width: 100%;
  padding: 8px 12px;
  background: #F0F0F0;
  border: none;
  border-radius: 15px;
  font-size: 13px;
}

.session-list .search-bar input:focus {
  outline: none;
  background: #E8E8E8;
}

.session-list .items {
  flex: 1;
  overflow-y: auto;
}

.session-item {
  padding: 10px 14px;
  display: flex;
  align-items: center;
  gap: 10px;
  cursor: pointer;
  border-left: 3px solid transparent;
}

.session-item:hover {
  background: #F5F5F5;
}

.session-item.active {
  background: var(--session-active);
  border-left-color: var(--primary);
}

.session-item .avatar {
  width: 36px;
  height: 36px;
  border-radius: 50%;
  display: flex;
  align-items: center;
  justify-content: center;
  color: white;
  font-size: 13px;
  flex-shrink: 0;
}

.session-item .avatar.group {
  border-radius: 6px;
  font-size: 11px;
}

.session-item .info {
  flex: 1;
  min-width: 0;
}

.session-item .info .top {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.session-item .info .name {
  font-weight: 600;
  font-size: 13px;
  color: var(--text-primary);
}

.session-item .info .time {
  font-size: 11px;
  color: var(--text-secondary);
}

.session-item .info .last-msg {
  font-size: 12px;
  color: var(--text-secondary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  margin-top: 2px;
}

/* Chat Area */
.chat-area {
  flex: 1;
  display: flex;
  flex-direction: column;
  background: var(--chat-bg);
}

.chat-area .header {
  padding: 14px 20px;
  background: white;
  border-bottom: 1px solid var(--border);
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.chat-area .header .title {
  font-weight: 600;
  font-size: 15px;
}

.chat-area .header .title .qq {
  color: var(--text-secondary);
  font-weight: 400;
  font-size: 12px;
  margin-left: 8px;
}

.chat-area .messages {
  flex: 1;
  overflow-y: auto;
  padding: 16px 20px;
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.message {
  display: flex;
  gap: 8px;
  max-width: 70%;
}

.message.self {
  align-self: flex-end;
  flex-direction: row-reverse;
}

.message .avatar {
  width: 32px;
  height: 32px;
  border-radius: 50%;
  display: flex;
  align-items: center;
  justify-content: center;
  color: white;
  font-size: 11px;
  flex-shrink: 0;
}

.message .bubble {
  padding: 8px 12px;
  border-radius: 0 12px 12px 12px;
  background: var(--bubble-other);
  color: var(--text-primary);
  font-size: 14px;
  line-height: 1.5;
  box-shadow: 0 1px 2px rgba(0,0,0,0.06);
}

.message.self .bubble {
  border-radius: 12px 0 12px 12px;
  background: var(--bubble-self);
  color: white;
}

.chat-area .input-area {
  background: white;
  border-top: 1px solid var(--border);
  padding: 8px 16px;
}

.chat-area .input-area .toolbar {
  display: flex;
  gap: 12px;
  margin-bottom: 8px;
  color: var(--text-secondary);
  font-size: 16px;
}

.chat-area .input-area .toolbar span {
  cursor: pointer;
}

.chat-area .input-area .input-row {
  display: flex;
  gap: 8px;
  align-items: flex-end;
}

.chat-area .input-area textarea {
  flex: 1;
  height: 60px;
  padding: 8px 12px;
  border: none;
  background: #F8F8F8;
  border-radius: 8px;
  resize: none;
  font-size: 14px;
  font-family: inherit;
}

.chat-area .input-area textarea:focus {
  outline: none;
  background: #F0F0F0;
}

.chat-area .input-area .send-btn {
  padding: 8px 20px;
  background: var(--primary);
  color: white;
  border: none;
  border-radius: 6px;
  cursor: pointer;
  font-size: 14px;
}

.chat-area .input-area .send-btn:hover {
  background: #0fa3dc;
}

/* Empty state */
.empty-state {
  flex: 1;
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--text-secondary);
  font-size: 14px;
}

/* Connection banner */
.connection-banner {
  position: fixed;
  top: 0;
  left: 0;
  right: 0;
  background: #FF6B6B;
  color: white;
  text-align: center;
  padding: 8px;
  font-size: 13px;
  z-index: 1000;
}
```

- [ ] **Step 2: Commit**

```bash
git add cmd/desktop/frontend/styles/
git commit -m "feat(desktop): add QQ blue theme CSS"
```

---

## Task 5: 前端 - 状态管理

**Files:**
- Create: `cmd/desktop/frontend/src/store.js`

- [ ] **Step 1: 创建简单状态管理**

```javascript
// store.js - 简单发布/订阅状态管理

class Store {
  constructor() {
    this.state = {
      currentUser: null,      // {qq, nickname}
      sessions: [],           // [{type, targetQQ, groupID, nickname, lastMessage, lastTime}]
      friends: [],            // [{qqNumber, nickname, groupName, online}]
      currentSession: null,   // {type, targetQQ, groupID}
      messages: [],           // [{id, fromQQ, toQQ, content, createdAt}]
      connected: false,
    };
    this.listeners = {};
  }

  get(key) {
    return this.state[key];
  }

  set(key, value) {
    this.state[key] = value;
    this.emit(key, value);
  }

  on(event, callback) {
    if (!this.listeners[event]) {
      this.listeners[event] = [];
    }
    this.listeners[event].push(callback);
  }

  emit(event, data) {
    if (this.listeners[event]) {
      this.listeners[event].forEach(cb => cb(data));
    }
  }
}

window.store = new Store();
```

- [ ] **Step 2: Commit**

```bash
git add cmd/desktop/frontend/src/store.js
git commit -m "feat(desktop): add simple state management"
```

---

## Task 6: 前端 - API 封装

**Files:**
- Create: `cmd/desktop/frontend/src/api.js`

- [ ] **Step 1: 创建 Wails bindings 封装**

```javascript
// api.js - Wails Go bindings 封装

import {Connect, Disconnect, Login, Register, SendMessage, SendGroupMessage, GetSessions, GetFriendList, GetHistory} from '../wailsjs/go/main/App.js';
import {EventsOn} from '../wailsjs/runtime/runtime.js';

export const api = {
  connect: (addr) => Connect(addr),
  disconnect: () => Disconnect(),
  login: (qq, password) => Login(qq, password),
  register: (nickname, password) => Register(nickname, password),
  sendMessage: (toQQ, content) => SendMessage(toQQ, content),
  sendGroupMessage: (groupID, content) => SendGroupMessage(groupID, content),
  getSessions: () => GetSessions(),
  getFriendList: () => GetFriendList(),
  getHistory: (targetQQ, offset, limit) => GetHistory(targetQQ, offset, limit),

  onLoginSuccess: (cb) => EventsOn("login-success", cb),
  onLoginFailed: (cb) => EventsOn("login-failed", cb),
  onRegisterSuccess: (cb) => EventsOn("register-success", cb),
  onRegisterFailed: (cb) => EventsOn("register-failed", cb),
  onSessionsUpdated: (cb) => EventsOn("sessions-updated", cb),
  onFriendsLoaded: (cb) => EventsOn("friends-loaded", cb),
  onHistoryLoaded: (cb) => EventsOn("history-loaded", cb),
  onMessageReceived: (cb) => EventsOn("message-received", cb),
  onMessageAck: (cb) => EventsOn("message-ack", cb),
  onConnectionLost: (cb) => EventsOn("connection-lost", cb),
};
```

- [ ] **Step 2: Commit**

```bash
git add cmd/desktop/frontend/src/api.js
git commit -m "feat(desktop): add Wails bindings API wrapper"
```

---

## Task 7: 前端 - 登录视图

**Files:**
- Create: `cmd/desktop/frontend/src/views/login.js`

- [ ] **Step 1: 创建登录/注册视图**

```javascript
// views/login.js

import {api} from '../api.js';

export function renderLogin() {
  const app = document.getElementById('app');
  app.innerHTML = `
    <div class="login-page">
      <div class="login-card">
        <h1>QQGO</h1>
        <div class="tabs">
          <div class="tab active" data-tab="login">登录</div>
          <div class="tab" data-tab="register">注册</div>
        </div>
        <div id="login-form">
          <input type="number" id="login-qq" placeholder="QQ号">
          <input type="password" id="login-password" placeholder="密码">
          <button id="login-btn">登录</button>
        </div>
        <div id="register-form" style="display:none">
          <input type="text" id="reg-nickname" placeholder="昵称">
          <input type="password" id="reg-password" placeholder="密码">
          <button id="reg-btn">注册</button>
        </div>
        <div class="server-addr">
          <label>服务端地址</label>
          <input type="text" id="server-addr" value="ws://localhost:8080/ws">
        </div>
      </div>
    </div>
  `;

  // Tab 切换
  document.querySelectorAll('.tab').forEach(tab => {
    tab.addEventListener('click', () => {
      document.querySelectorAll('.tab').forEach(t => t.classList.remove('active'));
      tab.classList.add('active');
      const isLogin = tab.dataset.tab === 'login';
      document.getElementById('login-form').style.display = isLogin ? 'block' : 'none';
      document.getElementById('register-form').style.display = isLogin ? 'none' : 'block';
    });
  });

  // 登录
  document.getElementById('login-btn').addEventListener('click', async () => {
    const addr = document.getElementById('server-addr').value;
    const qq = parseInt(document.getElementById('login-qq').value);
    const password = document.getElementById('login-password').value;

    if (!qq || !password) {
      alert('请输入 QQ 号和密码');
      return;
    }

    try {
      await api.connect(addr);
      await api.login(qq, password);
    } catch (err) {
      alert('连接失败: ' + err);
    }
  });

  // 注册
  document.getElementById('reg-btn').addEventListener('click', async () => {
    const addr = document.getElementById('server-addr').value;
    const nickname = document.getElementById('reg-nickname').value;
    const password = document.getElementById('reg-password').value;

    if (!nickname || !password) {
      alert('请输入昵称和密码');
      return;
    }

    try {
      await api.connect(addr);
      await api.register(nickname, password);
    } catch (err) {
      alert('连接失败: ' + err);
    }
  });

  // 监听登录成功
  api.onLoginSuccess((data) => {
    window.store.set('currentUser', {qq: data.qq, nickname: data.nickname});
    window.store.set('connected', true);
    window.showChat();
  });

  // 监听登录失败
  api.onLoginFailed((data) => {
    alert('登录失败: ' + data.message);
  });

  // 监听注册成功
  api.onRegisterSuccess((data) => {
    alert('注册成功！QQ号: ' + data.qq + '\n请切换到登录页登录');
  });

  // 监听注册失败
  api.onRegisterFailed((data) => {
    alert('注册失败: ' + data.message);
  });
}
```

- [ ] **Step 2: Commit**

```bash
git add cmd/desktop/frontend/src/views/login.js
git commit -m "feat(desktop): add login/register view"
```

---

## Task 8: 前端 - 主窗口组件

**Files:**
- Create: `cmd/desktop/frontend/src/components/sidebar.js`
- Create: `cmd/desktop/frontend/src/components/sessions.js`
- Create: `cmd/desktop/frontend/src/components/messages.js`
- Create: `cmd/desktop/frontend/src/views/chat.js`

- [ ] **Step 1: 创建侧边栏组件**

```javascript
// components/sidebar.js

export function renderSidebar() {
  const user = window.store.get('currentUser');
  const initial = user.nickname ? user.nickname[0].toUpperCase() : 'U';

  return `
    <div class="sidebar">
      <div class="avatar">${initial}</div>
      <div class="nav-item active" data-view="sessions">💬</div>
      <div class="nav-item" data-view="friends">👤</div>
      <div class="spacer"></div>
      <div class="nav-item" data-view="settings">⚙</div>
    </div>
  `;
}
```

- [ ] **Step 2: 创建会话列表组件**

```javascript
// components/sessions.js

export function renderSessions() {
  const sessions = window.store.get('sessions') || [];
  const current = window.store.get('currentSession');

  const items = sessions.map(s => {
    const isActive = current &&
      ((s.type === 'private' && current.targetQQ === s.targetQQ) ||
       (s.type === 'group' && current.groupID === s.groupID));

    const avatarClass = s.type === 'group' ? 'avatar group' : 'avatar';
    const avatarText = s.type === 'group' ? '群' : (s.nickname ? s.nickname[0] : '?');
    const avatarColor = s.type === 'group' ? '#6C5CE7' : stringToColor(s.nickname || '');
    const time = formatTime(s.lastTime);

    return `
      <div class="session-item ${isActive ? 'active' : ''}"
           data-type="${s.type}"
           data-target="${s.targetQQ || ''}"
           data-group="${s.groupID || ''}">
        <div class="${avatarClass}" style="background:${avatarColor}">${avatarText}</div>
        <div class="info">
          <div class="top">
            <span class="name">${s.nickname || s.targetQQ || s.groupID}</span>
            <span class="time">${time}</span>
          </div>
          <div class="last-msg">${s.lastMessage || ''}</div>
        </div>
      </div>
    `;
  }).join('');

  return `
    <div class="session-list">
      <div class="search-bar">
        <input type="text" placeholder="🔍 搜索">
      </div>
      <div class="items">${items || '<div class="empty-state">暂无会话</div>'}</div>
    </div>
  `;
}

function stringToColor(str) {
  const colors = ['#7BC67E', '#E8915A', '#6C5CE7', '#FD79A8', '#00B894', '#FDCB6E'];
  let hash = 0;
  for (let i = 0; i < str.length; i++) {
    hash = str.charCodeAt(i) + ((hash << 5) - hash);
  }
  return colors[Math.abs(hash) % colors.length];
}

function formatTime(timestamp) {
  if (!timestamp) return '';
  const date = new Date(timestamp * 1000);
  const now = new Date();
  const diff = now - date;

  if (diff < 60000) return '刚刚';
  if (diff < 3600000) return Math.floor(diff / 60000) + '分钟前';
  if (diff < 86400000) return date.getHours() + ':' + String(date.getMinutes()).padStart(2, '0');
  if (diff < 172800000) return '昨天';
  return (date.getMonth() + 1) + '/' + date.getDate();
}
```

- [ ] **Step 3: 创建消息区域组件**

```javascript
// components/messages.js

export function renderMessages() {
  const current = window.store.get('currentSession');
  const messages = window.store.get('messages') || [];
  const currentUser = window.store.get('currentUser');

  if (!current) {
    return '<div class="chat-area"><div class="empty-state">选择一个会话开始聊天</div></div>';
  }

  const title = current.nickname || current.targetQQ || current.groupID;
  const subtitle = current.type === 'private' ? `<span class="qq">${current.targetQQ}</span>` : '';

  const msgHtml = messages.map(m => {
    const isSelf = m.fromQQ === currentUser.qq;
    const avatarColor = isSelf ? '#12B7F5' : '#7BC67E';
    const avatarText = isSelf ? (currentUser.nickname ? currentUser.nickname[0] : '我') : (title ? title[0] : '?');

    return `
      <div class="message ${isSelf ? 'self' : ''}">
        <div class="avatar" style="background:${avatarColor}">${avatarText}</div>
        <div class="bubble">${escapeHtml(m.content)}</div>
      </div>
    `;
  }).join('');

  return `
    <div class="chat-area">
      <div class="header">
        <div class="title">${title} ${subtitle}</div>
      </div>
      <div class="messages" id="messages-container">${msgHtml}</div>
      <div class="input-area">
        <div class="toolbar">
          <span>😊</span>
          <span>📎</span>
          <span>📷</span>
        </div>
        <div class="input-row">
          <textarea id="message-input" placeholder="输入消息..."></textarea>
          <button class="send-btn" id="send-btn">发送</button>
        </div>
      </div>
    </div>
  `;
}

function escapeHtml(text) {
  const div = document.createElement('div');
  div.textContent = text;
  return div.innerHTML;
}

export function scrollToBottom() {
  const container = document.getElementById('messages-container');
  if (container) {
    container.scrollTop = container.scrollHeight;
  }
}
```

- [ ] **Step 4: 创建主窗口视图**

```javascript
// views/chat.js

import {api} from '../api.js';
import {renderSidebar} from '../components/sidebar.js';
import {renderSessions} from '../components/sessions.js';
import {renderMessages, scrollToBottom} from '../components/messages.js';

export function renderChat() {
  const app = document.getElementById('app');
  app.innerHTML = `
    <div class="main-window">
      ${renderSidebar()}
      ${renderSessions()}
      ${renderMessages()}
    </div>
  `;

  bindChatEvents();
  loadInitialData();
}

function bindChatEvents() {
  // 会话点击
  document.querySelectorAll('.session-item').forEach(item => {
    item.addEventListener('click', () => {
      const type = item.dataset.type;
      const targetQQ = parseInt(item.dataset.target) || 0;
      const groupID = item.dataset.group || '';

      const session = {type, targetQQ, groupID};
      const sessions = window.store.get('sessions') || [];
      const s = sessions.find(s =>
        (type === 'private' && s.targetQQ === targetQQ) ||
        (type === 'group' && s.groupID === groupID)
      );
      if (s) session.nickname = s.nickname;

      window.store.set('currentSession', session);
      window.store.set('messages', []);

      // 重新渲染
      renderChat();

      // 加载历史
      if (type === 'private') {
        api.getHistory(targetQQ, 0, 30);
      }
    });
  });

  // 发送消息
  const sendBtn = document.getElementById('send-btn');
  const input = document.getElementById('message-input');

  if (sendBtn && input) {
    const send = () => {
      const content = input.value.trim();
      if (!content) return;

      const current = window.store.get('currentSession');
      if (!current) return;

      if (current.type === 'private') {
        api.sendMessage(current.targetQQ, content);
      } else {
        api.sendGroupMessage(current.groupID, content);
      }

      // 立即显示自己的消息
      const messages = window.store.get('messages') || [];
      messages.push({
        fromQQ: window.store.get('currentUser').qq,
        content: content,
        createdAt: Math.floor(Date.now() / 1000),
      });
      window.store.set('messages', messages);

      input.value = '';
      renderChat();
      setTimeout(scrollToBottom, 50);
    };

    sendBtn.addEventListener('click', send);
    input.addEventListener('keydown', (e) => {
      if (e.key === 'Enter' && !e.shiftKey) {
        e.preventDefault();
        send();
      }
    });
  }
}

function loadInitialData() {
  api.getSessions();
  api.getFriendList();
}

// 监听事件
api.onSessionsUpdated((sessions) => {
  window.store.set('sessions', sessions);
  if (document.querySelector('.main-window')) {
    renderChat();
  }
});

api.onFriendsLoaded((friends) => {
  window.store.set('friends', friends);
});

api.onHistoryLoaded((messages) => {
  window.store.set('messages', messages);
  if (document.querySelector('.main-window')) {
    renderChat();
    setTimeout(scrollToBottom, 50);
  }
});

api.onMessageReceived((msg) => {
  const current = window.store.get('currentSession');
  const messages = window.store.get('messages') || [];

  // 如果是当前会话的消息，添加到列表
  if (current &&
      ((current.type === 'private' && (msg.fromQQ === current.targetQQ || msg.toQQ === current.targetQQ)) ||
       (current.type === 'group' && msg.groupID === current.groupID))) {
    messages.push(msg);
    window.store.set('messages', messages);
    if (document.querySelector('.main-window')) {
      renderChat();
      setTimeout(scrollToBottom, 50);
    }
  }

  // 刷新会话列表
  api.getSessions();
});

api.onConnectionLost(() => {
  window.store.set('connected', false);
  showBanner('连接已断开，正在重连...');
});
```

- [ ] **Step 5: Commit**

```bash
git add cmd/desktop/frontend/src/
git commit -m "feat(desktop): add main window with sidebar, sessions, and chat components"
```

---

## Task 9: 前端 - 应用入口

**Files:**
- Create: `cmd/desktop/frontend/src/main.js`
- Modify: `cmd/desktop/frontend/index.html`

- [ ] **Step 1: 创建应用入口**

```javascript
// main.js

import './store.js';
import {renderLogin} from './views/login.js';
import {renderChat} from './views/chat.js';

window.showChat = renderChat;

// 启动时显示登录页
renderLogin();
```

- [ ] **Step 2: 修改 index.html**

```html
<!DOCTYPE html>
<html lang="zh-CN">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>QQGO</title>
  <link rel="stylesheet" href="./styles/main.css">
</head>
<body>
  <div id="app"></div>
  <script type="module" src="./src/main.js"></script>
</body>
</html>
```

- [ ] **Step 3: 验证应用可运行**

```bash
cd /Users/yangshikang.6/Desktop/Code/Go/QQGO/cmd/desktop
wails dev
```

Expected: 打开窗口显示登录页面

- [ ] **Step 4: Commit**

```bash
git add cmd/desktop/frontend/
git commit -m "feat(desktop): add application entry point and HTML"
```

---

## Task 10: 集成测试

**Files:**
- 无新文件

- [ ] **Step 1: 启动服务端**

```bash
cd /Users/yangshikang.6/Desktop/Code/Go/QQGO
go run cmd/server/main.go
```

Expected: 服务端启动在 :8080

- [ ] **Step 2: 启动桌面端**

```bash
cd /Users/yangshikang.6/Desktop/Code/Go/QQGO/cmd/desktop
wails dev
```

Expected: 桌面端窗口打开

- [ ] **Step 3: 测试注册流程**

1. 在桌面端输入昵称和密码
2. 点击注册
3. 验证收到 QQ 号

Expected: 显示注册成功和分配的 QQ 号

- [ ] **Step 4: 测试登录流程**

1. 切换到登录 Tab
2. 输入 QQ 号和密码
3. 点击登录

Expected: 跳转到主窗口，显示会话列表

- [ ] **Step 5: 测试消息收发**

1. 同时启动 CLI 客户端并登录另一个账号
2. 在桌面端发送消息给 CLI 客户端
3. 在 CLI 客户端回复消息

Expected: 双方都能收到对方消息，界面实时更新

- [ ] **Step 6: Commit**

```bash
git add .
git commit -m "test(desktop): integration test passed"
```

---

## 完成标准

- [ ] 所有 Task 完成
- [ ] 桌面端可成功登录
- [ ] 可发送和接收私聊消息
- [ ] 会话列表实时更新
- [ ] 历史记录正确加载
- [ ] 断线后显示横幅提示
