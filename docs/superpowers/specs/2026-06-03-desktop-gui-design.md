# QQGO 桌面端 GUI 设计文档

**版本：** v0.13
**日期：** 2026-06-03
**状态：** 待审批

---

## 1. 概述

为 QQGO 开发桌面端 GUI 客户端，参考 QQ 桌面端风格，使用 Wails v2 框架（Go + Web 前端），通过 WebSocket + Protobuf 连接现有服务端。

### 1.1 决策记录

| 决策项 | 选择 | 理由 |
|--------|------|------|
| 框架 | Wails v2 | 轻量（~10MB）、UI 灵活、Go 原生集成 |
| 布局 | QQ 经典三栏 | 最接近 QQ 桌面端体验 |
| 配色 | QQ 蓝 (#12B7F5) | QQ 标志性配色 |
| 通信 | WebSocket + Protobuf | 和 CLI 客户端共享服务端 |
| MVP 范围 | 登录/注册 + 好友列表 + 私聊 + 基础群聊 | 先跑通核心流程 |

---

## 2. MVP 功能范围

### 2.1 包含

- 登录/注册（QQ号+密码 / 昵称+密码）
- 会话列表（私聊 + 群聊，按最后消息时间排序）
- 私聊消息收发（文本）
- 群聊消息收发（文本）
- 好友列表展示（只读）
- 历史记录加载（进入会话时拉取）
- 断线重连（3s 间隔，最多 5 次）
- 离线消息推送（登录后自动拉取）

### 2.2 不包含（后续迭代）

- 好友管理操作（添加/删除/分组/备注）
- 文件传输
- 消息撤回/已读
- 搜索消息
- 修改密码/黑名单
- 备份/清理
- 图片/语音消息
- 通知推送（系统托盘）

---

## 3. 架构

### 3.1 目录结构

```
cmd/desktop/
├── main.go                 # Wails 入口，窗口配置
├── app.go                  # Go 后端：WebSocket 客户端 + Wails 绑定
├── frontend/
│   ├── index.html
│   ├── package.json
│   ├── src/
│   │   ├── main.js         # 应用初始化，路由（登录/主窗口）
│   │   ├── ws.js           # Wails Events 监听封装
│   │   ├── store.js        # 状态管理（简单发布/订阅）
│   │   ├── views/
│   │   │   ├── login.js    # 登录/注册页
│   │   │   └── chat.js     # 主窗口三栏
│   │   ├── components/
│   │   │   ├── sidebar.js  # 左侧图标导航
│   │   │   ├── sessions.js # 会话列表
│   │   │   └── message.js  # 聊天区域 + 输入框
│   │   └── styles/
│   │       └── main.css    # QQ 蓝配色主题
│   └── wailsjs/            # Wails 自动生成的 Go 绑定
```

### 3.2 数据流

```
前端 JS ←→ Wails Bindings ←→ Go app.go ←→ WebSocket + Protobuf ←→ 服务端
```

- Go 层管理 WebSocket 连接生命周期
- 前端通过 Wails 绑定调用 Go 方法
- Go 收到服务端消息后通过 Wails Events 推送到前端

### 3.3 Protobuf 复用

- 直接 import `internal/protocol` 包
- Go 层 marshal/unmarshal protobuf，前端收发 JSON
- 前端不需要 protobuf 库

---

## 4. 界面设计

### 4.1 登录/注册页

- 居中卡片布局，QQGO Logo
- 登录 Tab：QQ号 + 密码 + 登录按钮
- 注册 Tab：昵称 + 密码 + 注册按钮
- 登录成功 → 跳转主窗口
- 底部显示服务端地址输入框（默认 `ws://localhost:8080/ws`）

### 4.2 主窗口（三栏）

**左栏 — 图标导航（54px 宽）：**

| 图标 | 功能 | MVP |
|------|------|-----|
| 头像 | 当前用户信息 | 展示 QQ 号 |
| 💬 | 消息（会话列表） | ✅ |
| 👤 | 联系人（好友列表） | 只读展示 |
| ⚙ | 设置 | 仅退出登录 |

- 背景色：#2B2F3A（深色）
- 图标未选中：#8E94A5
- 图标选中/强调：#12B7F5

**中栏 — 会话列表（240px 宽）：**

- 顶部：搜索框（圆角，灰色背景）+ 加号按钮（MVP 暂不实现）
- 会话项：圆形头像（36px）+ 昵称 + 最后消息摘要 + 时间
- 选中态：左侧 3px 蓝色边框 + 浅蓝背景 (#E6F7FF)
- 群聊用方形头像（6px 圆角）区分私聊

**右栏 — 聊天区域：**

- 顶部栏：对方昵称 + QQ号 + 操作按钮（MVP 暂不实现语音/视频）
- 消息区：
  - 对方消息：白底气泡，左对齐，圆角 `0 12px 12px 12px`
  - 自己消息：蓝底白字气泡，右对齐，圆角 `12px 0 12px 12px`
  - 头像 32px 圆形，紧跟气泡
  - 背景色：#F5F5F5
- 输入区：
  - 工具栏：表情/附件/图片图标（MVP 仅展示，不实现）
  - 输入框：多行文本，高度 60px
  - 发送按钮：蓝色 #12B7F5
  - Enter 发送，Shift+Enter 换行

### 4.3 配色方案

| 用途 | 颜色 |
|------|------|
| 主色（按钮、强调、自己消息） | #12B7F5 |
| 深色侧栏 | #2B2F3A |
| 侧栏图标（默认） | #8E94A5 |
| 会话列表背景 | #FFFFFF |
| 会话选中背景 | #E6F7FF |
| 聊天区背景 | #F5F5F5 |
| 对方消息气泡 | #FFFFFF |
| 文字主色 | #333333 |
| 文字次色 | #999999 |
| 边框 | #E5E5E5 |

---

## 5. Go 后端 API

### 5.1 Wails 绑定方法（前端 → Go）

| 方法 | 参数 | 返回 | 说明 |
|------|------|------|------|
| `Connect(addr string)` | WebSocket 地址 | `error` | 建立连接 |
| `Login(qq int64, password string)` | QQ号+密码 | `error` | 密码登录 |
| `Register(nickname, password string)` | 昵称+密码 | `error` | 注册 |
| `SendMessage(toQQ int64, content string)` | 目标+内容 | `error` | 发送私聊 |
| `SendGroupMessage(groupID, content string)` | 群ID+内容 | `error` | 发送群消息 |
| `GetSessions()` | 无 | `error` | 请求会话列表 |
| `GetHistory(targetQQ, offset, limit)` | 目标+分页 | `error` | 请求历史记录 |
| `GetFriendList()` | 无 | `error` | 请求好友列表 |
| `Disconnect()` | 无 | 无 | 断开连接 |

### 5.2 Wails Events（Go → 前端）

| 事件 | 数据 | 触发时机 |
|------|------|----------|
| `login-success` | `{qq, nickname, accessToken}` | 登录/注册成功 |
| `login-failed` | `{message}` | 登录失败 |
| `sessions-updated` | `[{type, targetQQ, groupID, nickname, lastMessage, lastTime}]` | 会话列表更新 |
| `friends-loaded` | `[{qqNumber, nickname, groupName, online}]` | 好友列表加载 |
| `history-loaded` | `[{id, fromQQ, toQQ, content, createdAt}]` | 历史记录加载 |
| `message-received` | `{id, fromQQ, toQQ, groupID, content, createdAt}` | 收到新消息 |
| `message-ack` | `{id, clientSeq}` | 发送确认 |
| `connection-lost` | 无 | WebSocket 断开 |
| `connection-restored` | 无 | 重连成功 |

---

## 6. 交互流程

### 6.1 启动流程

```
启动应用 → 登录页
  → 输入服务端地址（默认 ws://localhost:8080/ws）
  → 输入 QQ号 + 密码（或注册）
  → Connect() → Login()
  → 收到 login-success → 跳转主窗口
  → GetSessions() + GetFriendList()
  → 收到 sessions-updated + friends-loaded → 渲染列表
```

### 6.2 聊天流程

```
点击会话项 → GetHistory(targetQQ, 0, 30)
  → 收到 history-loaded → 渲染消息气泡
  → 用户输入 → Enter → SendMessage()
  → 收到 message-ack → 标记消息已发送
  → 收到 message-received → 追加到消息区 → 自动滚动到底部
  → 会话列表更新（最后消息 + 时间）
```

### 6.3 断线重连

```
WebSocket 断开 → connection-lost → 顶部显示"连接已断开"横幅
  → 3s 后自动重连（最多 5 次）
  → 重连成功 → connection-restored → 重新 Login → 拉取离线消息
  → 5 次失败 → 显示"无法连接，请检查服务端"
```

---

## 7. WebSocket 连接管理

### 7.1 Go 层实现

- 使用 `gorilla/websocket` 客户端（和 CLI 客户端一致）
- 独立 goroutine 做 ReadLoop
- 心跳 30s 自动发送 Heartbeat
- 收到消息后根据 protobuf payload 类型分发
- 通过 `runtime.EventsEmit(eventName, data...)` 推给前端

### 7.2 重连策略

- 指数退避：3s → 6s → 12s → 24s → 48s（上限 5 次）
- 重连后自动用保存的 QQ号+密码重新登录
- 登录成功后拉取离线消息

---

## 8. 窗口配置

| 属性 | 值 |
|------|-----|
| 标题 | QQGO |
| 默认宽度 | 960px |
| 默认高度 | 640px |
| 最小宽度 | 720px |
| 最小高度 | 480px |
| 可调整大小 | 是 |
| 全屏 | 否 |

---

## 9. 依赖

### 9.1 新增

| 依赖 | 用途 |
|------|------|
| `github.com/wailsapp/wails/v2` | 桌面应用框架 |

### 9.2 复用

| 依赖 | 用途 |
|------|------|
| `github.com/gorilla/websocket` | WebSocket 客户端 |
| `google.golang.org/protobuf` | Protobuf 序列化 |
| `internal/protocol` | 消息类型定义 |
| `internal/model` | 数据模型 |

---

## 10. 测试策略

- **Go 层**：单元测试覆盖 WebSocket 消息收发逻辑（mock WebSocket）
- **前端**：手动测试为主，验证各界面状态切换
- **集成**：启动服务端 + 桌面端，端到端验证登录→发消息→收到

---

## 11. 不包含

- 好友管理操作（添加/删除/分组/备注）
- 文件/图片/语音传输
- 消息撤回/已读回执
- 全文搜索
- 修改密码/黑名单
- 数据库备份/清理
- 系统托盘通知
- 多账号同时登录
- 暗色模式切换
