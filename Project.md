# QQGO — 即时通讯项目

## 版本 0.1 — 2026-05-15

基于 Go 语言的轻量级即时通讯系统，支持多用户终端聊天。

---

### 项目目录结构

```
QQGO/
├── Project.md                     # 项目文档（本文件）
├── go.mod                         # Go Module 定义
├── cmd/
│   ├── server/main.go             # 服务端入口
│   └── client/main.go             # CLI 客户端入口
├── internal/
│   ├── config/config.go           # 环境变量配置
│   ├── model/
│   │   ├── message.go             # 消息/协议类型定义
│   │   └── user.go                # 用户/好友/群组模型
│   ├── handler/ws.go              # WebSocket Hub — 连接管理/消息路由
│   ├── service/chat.go            # 聊天服务（消息存储/校验）
│   ├── middleware/                 # 中间件（预留）
│   ├── protocol/                  # 协议定义（预留）
│   └── store/                     # 存储层（预留）
├── pkg/
│   └── websocket/conn.go          # WebSocket 连接封装（心跳/读写协程）
└── deployments/                   # 部署配置（预留）
```

---

### 技术栈

| 组件 | 选型 |
|------|------|
| 语言 | Go 1.26 |
| 通信协议 | WebSocket（`gorilla/websocket`） |
| 序列化 | JSON |
| 存储（当前） | 内存 |

---

### 已实现功能

| 模块 | 功能 | 说明 |
|------|------|------|
| 服务端 | WebSocket 连接管理 | 多用户并行接入，连接断开自动清理 |
| 服务端 | 登录认证 | 基于 UID + Token 的登录流程（当前 Token 校验为放行模式） |
| 服务端 | 点对点消息 | 文本消息实时转发 |
| 服务端 | Hub 路由 | 按 UID 查找连接并投递消息 |
| 服务端 | 心跳保活 | 服务端主动 Ping，客户端自动 Pong |
| 服务端 | 消息持久化 | 内存队列存储（最多 10 万条） |
| 客户端 | 终端 UI | CLI 交互式聊天 |
| 客户端 | 命令系统 | `/to` 切换聊天对象、`/who` 查看目标、`/help` 帮助、`/quit` 退出 |
| 客户端 | 登录 | 启动时传入 UID 自动登录 |
| 通用 | 日志系统 | 服务端/客户端关键路径日志 |

---

### 使用方法

#### 1. 启动服务端

```bash
cd QQGO
go run ./cmd/server
```

输出：
```
QQGO server starting on 0.0.0.0:8080
```

#### 2. 启动客户端（多个终端）

```bash
# 终端 2 — 用户 alice
go run ./cmd/client alice

# 终端 3 — 用户 bob
go run ./cmd/client bob
```

#### 3. 开始聊天

```
# alice 的终端
(no target) > /to bob
[bob] > hello bob!

# bob 的终端（需要先 /to alice）
[alice] > hello bob!      # 收到消息
[alice] > hi alice!       # 发送回复
```

#### 4. 切换聊天对象

```
[bob] > /to alice         # 切到 alice
[alice] > /who            # 查看当前对象
[cmd] chatting with alice
```

---

### 后续开发需求（按优先级排序）

以下为后续版本计划引入的功能，按紧迫程度排序：

#### P0 — 核心基建（下个版本优先）

| 需求 | 说明 |
|------|------|
| **数据库接入** | 引入 SQLite/PostgreSQL，持久化用户、好友关系、聊天记录 |
| **用户注册/登录** | 完善注册流程（密码哈希），替换当前放行模式的 Token 校验 |
| **离线消息** | 目标不在线时消息写入数据库，上线后拉取历史 |
| **消息可靠性** | ACK 确认机制 + 消息序号，保证不丢不重 |

#### P1 — 功能完善

| 需求 | 说明 |
|------|------|
| **群组聊天** | 创建群、加群、群消息广播 |
| **好友管理** | 搜索/添加/删除好友，好友列表 |
| **消息类型扩展** | 图片、文件、语音消息 |
| **历史消息查询** | 按时间/会话拉取历史记录 |

#### P2 — 体验与架构

| 需求 | 说明 |
|------|------|
| **桌面端 GUI** | 引入 Wails（Go + Web 前端）或 Fyne 开发桌面客户端 |
| **Protobuf 协议** | 替换 JSON 序列化，降低带宽 |
| **分布式扩展** | 接入 NATS/Redis PubSub 实现多网关消息路由 |
| **在线状态** | Redis 缓存在线状态，支持状态变更通知 |
| **消息已读** | 已读回执、未读计数 |

#### P3 — 运维与安全

| 需求 | 说明 |
|------|------|
| **TLS/SSL** | WebSocket 加密传输 |
| **限流/鉴权** | 接口限流、JWT Token |
| **Docker 部署** | Dockerfile + docker-compose 一键启动 |
| **单元测试** | 核心模块测试覆盖 |

---

### 环境变量

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `SERVER_HOST` | `0.0.0.0` | 服务端监听地址 |
| `SERVER_PORT` | `8080` | 服务端监听端口 |
| `MAX_CONNECTIONS` | `10000` | 最大并发连接数 |
| `REDIS_ADDR` | `localhost:6379` | Redis 地址（预留） |
| `POSTGRES_DSN` | `postgres://...` | 数据库连接串（预留） |
| `NATS_URL` | `nats://localhost:4222` | NATS 地址（预留） |
| `DB_PATH` | `./qqgo.db` | SQLite 数据库文件路径 |
| `BCRYPT_COST` | `12` | bcrypt 密码哈希轮次 |

---

## 版本 0.2 — 2026-05-19 开发中

### P0 功能实现

本版本聚焦核心基建，解决 v0.1 的四个关键缺失。

---

### 1. 数据库接入

| 项目 | 选型 |
|------|------|
| 数据库 | SQLite（通过 `modernc.org/sqlite` 纯 Go 驱动） |
| ORM | GORM（`gorm.io/gorm`） |
| 文件 | `./qqgo.db`（可通过 `DB_PATH` 环境变量配置） |

**建表（GORM AutoMigrate）：**

| 表 | 对应 Model | 说明 |
|----|-----------|------|
| `users` | `User` | 用户账号、密码哈希、昵称 |
| `friends` | `Friend` | 好友关系 |
| `groups` | `Group` | 群组 |
| `group_members` | `GroupMember` | 群成员 |
| `messages` | `Message` | 消息主表（含离线消息，用 `delivered` 标记） |

---

### 2. 用户注册/登录

| 环节 | 实现 |
|------|------|
| 密码哈希 | `golang.org/x/crypto/bcrypt`，默认 cost=12 |
| 注册 | `/register` 命令：客户端发送 `uid + password + nickname`，服务端 bcrypt 哈希后写入 users 表 |
| 登录 | 客户端发送 `uid + password`，服务端 bcrypt.Compare 校验，成功返回 token |
| Token | SHA256 随机字符串，存入 users 表 `token` 字段，后续请求携带 token 验证 |

**新增消息类型：**
- `MsgTypeRegister = 105` — 注册请求
- `MsgTypeRegisterAck = 106` — 注册响应

---

### 3. 消息持久化 + 离线消息

**messages 表结构（GORM Model）：**

```go
type Message struct {
    ID         int64     `gorm:"primaryKey;autoIncrement"`  // server_seq
    MsgType    int32     `gorm:"not null"`
    FromUID    string    `gorm:"index;not null"`
    ToUID      string    `gorm:"index;not null"`
    GroupID    string    `gorm:"index"`
    Content    string    `gorm:"not null"`
    Delivered  bool      `gorm:"default:false"`              // false=离线待投递
    CreatedAt  time.Time
}
```

**离线消息流程：**
1. 服务端收到消息 → `delivered=false` 入库 → 返回 ACK 给发送方
2. 目标在线 → 转发消息 → 收到 DELIVERED ACK → `delivered=true`
3. 目标离线 → 消息保留在表中
4. 目标上线 → 查询 `to_uid = ? AND delivered = false` → 批量推送 → 标记为已投递

---

### 4. ACK 消息确认机制

采用 **服务端 seq 单序号模型**（适配当前单机架构）：

```
Sender                    Server                     Receiver
  |                         |                          |
  |-- msg ---------------->|                          |
  |                      [分配 ID, 入库]               |
  |<-- ACK {id:1098} -----|                          |
  |                         |-- msg {id:1098} ------->|
  |                         |<-- ACK {id:1098} -------|
  |<-- DELIVERED ----------|                          |
```

| 消息类型 | 含义 |
|----------|------|
| `MsgTypeServerAck = 107` | 服务端已接收并持久化（发给发送方） |
| `MsgTypeDelivered = 108` | 消息已送达目标（发给发送方） |

**新增消息类型汇总：**

| 常量 | 值 | 说明 |
|------|-----|------|
| `MsgTypeRegister` | 105 | 注册请求 |
| `MsgTypeRegisterAck` | 106 | 注册响应 |
| `MsgTypeServerAck` | 107 | 服务端确认已接收 |
| `MsgTypeDelivered` | 108 | 消息已送达对方 |

---

### 目录结构变更

```
QQGO/
├── internal/
│   ├── config/config.go           # 新增 DB_PATH 配置
│   ├── model/
│   │   ├── message.go             # 修改：增加 Delivered 字段、新消息类型
│   │   └── user.go                # 修改：User 增加 PasswordHash/Token
│   ├── handler/ws.go              # 修改：新增 ACK/离线消息/注册处理
│   ├── service/
│   │   ├── chat.go                # 修改：接入 GORM，真实 token 校验
│   │   └── auth.go                # 新增：注册/登录逻辑
│   ├── store/
│   │   └── db.go                  # 新增：GORM 初始化 + AutoMigrate
│   └── middleware/                # （仍预留）
├── cmd/
│   ├── server/main.go             # 修改：初始化数据库
│   └── client/main.go             # 修改：支持注册/ACK 显示
├── go.mod                         # 新增依赖
└── qqgo.db                        # SQLite 数据文件（运行时生成）
```

---

### 环境变量（v0.2 新增）

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `DB_PATH` | `./qqgo.db` | SQLite 数据库文件路径 |
| `BCRYPT_COST` | `12` | bcrypt 密码哈希轮次 |


---

## 版本 0.3 — 2026-05-20 已完成

### 好友系统

基于 QQ 号唯一标识的完整好友关系系统。

---

### 1. QQ 号系统

| 项目 | 说明 |
|------|------|
| 生成规则 | `QQNumber = 10000 + auto_increment ID` |
| 起始值 | 10001（第一个注册用户） |
| 特性 | 不可变，注册时自动分配 |
| 存储 | `users.qq_number` 列（唯一索引） |

注册/登录成功时返回 QQ 号：
```
[Server]: register ok, your QQ number is 10001
[Server]: login ok, QQ=10001, online=1
```

---

### 2. Friend 模型

```go
type Friend struct {
    UID       string  // 发起方
    FriendUID string  // 对方
    Remark    string  // 备注名
    GroupName string  // 分组（默认"我的好友"）
    Status    int     // 0=待接受, 1=已接受, 2=已拒绝
}
```

同一张 `friends` 表覆盖待处理请求 + 已确认好友 + 已拒绝。

---

### 3. 客户端命令

| 命令 | 说明 |
|------|------|
| `/addfriend <qq> [msg]` | 发送好友申请 |
| `/accept <qq>` | 接受好友申请 |
| `/reject <qq>` | 拒绝好友申请 |
| `/delfriend <qq>` | 删除好友 |
| `/friends` | 列出好友（按分组显示） |
| `/search <keyword>` | 搜索用户（QQ号/昵称） |
| `/movefriend <qq> <group>` | 移动好友到分组 |
| `/groups` | 列出所有分组 |
| `/remark <qq> <remark>` | 设置备注名 |

---

### 4. 消息类型（新增）

| 常量 | 值 | 说明 |
|------|-----|------|
| `MsgTypeFriendReject` | 302 | 拒绝好友 |
| `MsgTypeFriendDelete` | 303 | 删除好友 |
| `MsgTypeFriendList` | 304 | 好友列表 |
| `MsgTypeFriendSearch` | 305 | 搜索用户 |
| `MsgTypeFriendMoveGroup` | 306 | 移动分组 |
| `MsgTypeFriendRemark` | 307 | 设置备注 |
| `MsgTypeFriendGroups` | 308 | 分组列表 |

---

### 5. 业务规则

| 规则 | 说明 |
|------|------|
| 好友上限 | 500（双方均不能超额） |
| 不能添加自己 | 检测 QQ 号是否相同 |
| 重复请求 | 已存在 pending/accepted 记录时拒绝 |
| 双向删除 | 删除好友时同时删除双向记录 |
| 接受好友 | 自动创建双向 Friend 记录（status=accepted） |
| 在线通知 | 好友请求/接受实时推送给在线用户 |
| 离线请求 | 好友请求持久化，上线后通过 `/friends` 可见 |
| 默认分组 | "我的好友"（未分组时的默认值） |

---

### 6. 交互流程

```
Alice(10001) 注册 → 得到 QQ=10001
Bob(10002) 注册   → 得到 QQ=10002

Alice: /search Bob              → QQ:10002  Bob  [bob]
Alice: /addfriend 10002 hello   → 服务端创建 pending，通知 Bob
Bob:   收到实时通知               → [Friend Request] from 10001(Alice): hello
Bob:   /friends                 → [待处理] 10001 Alice [pending]
Bob:   /accept 10001            → 双向记录更新为 accepted
Alice: /friends                 → [我的好友] ● QQ:10002 Bob [bob]
Alice: /remark 10002 Bobby      → 备注设置为 Bobby
Alice: /movefriend 10002 家人    → 移动到"家人"分组
Alice: /friends                 → [家人] ● QQ:10002 Bobby(Bob) [bob]
```

---

### 7. 好友列表显示格式

```
───── Friend List ─────

  [待处理]
    ● QQ:10003  Charlie  [charlie]

  [我的好友]
    ● QQ:10002  Bob  [bob]
    ○ QQ:10004  Dave  [dave]

  [家人]
    ● QQ:10005  Bobby(Bob2)  [bob2]

──────────────────────
```

● 在线  /  ○ 离线  /  括号内为备注名