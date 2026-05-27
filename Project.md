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

### 后续开发需求

详见 [REQUIREMENTS.md](./REQUIREMENTS.md)

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

---

## 测试记录：2026-05-21 第一次测试

### 通过项

| 功能 | 结果 |
|------|------|
| 注册 + QQ 号分配 | 通过 |
| 登录认证（bcrypt） | 通过 |
| 点对点消息 + ACK（sent ✓ / delivered ✓✓） | 通过 |
| 离线消息上线自动推送 | 通过 |
| 好友添加/接受/拒绝/删除 | 通过 |
| 好友搜索（QQ号 + 昵称） | 通过 |
| 好友备注 `/remark` | 通过 |
| 好友分组 `/movefriend` `/groups` | 通过（存在设计问题，见下方） |
| 数据持久化（重启不丢） | 通过 |

### 发现问题

| 编号 | 类型 | 描述 | 优先级 | 关联 |
|------|------|------|--------|------|
| 1 | bug | `/to` 不存在用户时消息被存储（BUG-007） | P0 | — |
| 2 | bug | `/quit` 退出时错误日志（BUG-001, BUG-008） | P1 | 需优雅退出 |
| 3 | design | 好友分组应先创建再移动，不应自动创建 | P1 | REQUIREMENTS v0.4-4 |
| 4 | feature | 每次改 schema 需删数据库，需要迁移方案 | P1 | REQUIREMENTS v0.4-8 |
| 5 | feature | 只能给已存在用户 / 好友发消息 | P1 | REQUIREMENTS v0.4-5 |
| 6 | feature | 客户端内 `/login` 命令，无需重启 | P1 | REQUIREMENTS v0.4-6 |
| 7 | feature | `/whoami` 查看当前账号信息 | P1 | REQUIREMENTS v0.4-7 |
| 8 | feature | 数据库管理接口（导出/备份） | P2 | REQUIREMENTS 待讨论 |
| 9 | note | 备注功能已在 v0.3 实现（`/remark`），无需重新开发 | — | — |

---

## 版本 0.5 — 2026-05-25 已完成

### 非好友消息限制

| 项目 | 说明 |
|------|------|
| 规则 | 好友自由发消息，非好友只能发 1 条 |
| 存储 | `message_counts` 表：`(from_qq, to_qq, count)` |
| 计数清零 | 接受好友请求时清除双方 message_counts |
| 删除好友 | 删除后恢复非好友限制 |

### 会话历史记录

| 项目 | 说明 |
|------|------|
| 触发 | `/to <qq>` 成功后自动拉取 |
| 默认 | 最近 30 条，按时间升序 |
| 翻页 | `/prev` 更早，`/next` 更新 |
| 显示 | `[我]` / `[对方昵称]` + 时间戳 + 内容 |

### 群组聊天

| 命令 | 说明 |
|------|------|
| `/mkgroup <name>` | 创建群 |
| `/joingroup <group_id>` | 加入群 |
| `/leavegroup <group_id>` | 退出群（群主不能退出） |
| `/mygroups` | 我的群列表 |
| `/togroup <group_id>` | 切换到群聊 |

群消息广播给所有成员，非成员不能发群消息。

### 会话列表

| 命令 | 说明 |
|------|------|
| `/sessions` | 列出所有私聊和群聊会话 |

显示：类型、目标 QQ/群 ID、昵称、最后消息、时间。按最后消息时间倒序。

---

## 版本 0.6 — 2026-05-25 规划中

### 规划来源

本版本需求来自：
1. v0.5 手动测试反馈（TEST.md 2026-05-25）
2. 历史遗留未规划需求（Token 持久化）
3. 历史未测试 BUG（BUG-005、BUG-006）

### P0 — BUG 修复

| BUG | 说明 | 优先级 |
|-----|------|--------|
| BUG-009 | 接收方消息打印到 prompt 中间 | ✅ fixed |
| BUG-010 | `/leavegroup` 后无法退出群聊窗口 | ✅ fixed |

### P1 — 功能完善

| 需求 | 说明 |
|------|------|
| **群成员发送前校验** | ✅ 服务端返回 "not group member" 时自动退出群聊窗口 |
| **群聊历史记录** | ✅ 群聊支持 `/prev` `/next` 翻页，`/togroup` 后自动拉取 |
| **本地聊天日志** | ✅ 客户端按 QQ 号分目录保存聊天记录到 `DATA/` 目录 |
| **Token 持久化客户端** | ✅ 登录保存 token，启动自动 Token 登录，`/logout` 清除 |

### P2 — 待测试

| 项 | 说明 |
|----|------|
| BUG-005 | 好友上限 500 边界测试 |
| BUG-006 | 离线好友请求持久化端到端测试 |

---

## 版本 0.7 — 2026-05-26 已完成

### 核心功能补全 & 体验完善

聚焦 P1/P2 级功能，补齐现代 IM 基础能力。

### P0 — BUG 修复

| 项 | 说明 | 状态 |
|----|------|------|
| **Server 优雅退出** | `srv.Close()` → `srv.Shutdown(ctx)` + Hub 清理 + DB 关闭；Ctrl+C 端口立即释放 | ✅ fixed |
| **SQLite GREATEST 兼容** | `LeaveGroup` 中 `GREATEST()` → `MAX()`；消除 SQLite 函数不支持报错 | ✅ fixed |

### P1 — 测试补全

| 项 | 说明 | 状态 |
|----|------|------|
| **BUG-005** | 好友上限 500 边界测试（TestFriendLimit500） | ✅ tested |
| **BUG-006** | 离线好友请求持久化 E2E 测试（TestOfflineFriendRequest） | ✅ tested |

### P2 — 新功能

| 需求 | 说明 | 状态 |
|------|------|------|
| **修改密码** | `/changepw <old> <new>`，旧密码校验 + Token 刷新 | ✅ done |
| **黑名单** | `blacklists` 表 + `/block` `/unblock` `/blacklist` + 消息拒绝 | ✅ done |
| **图片/文件消息** | `/sendimg` `/sendfile` + base64 传输（≤5MB）+ 自动保存 | ✅ done |
| **消息已读/未读** | `read_at` 字段 + 自动已读回执 | ✅ done |
| **消息撤回** | `is_recalled` 字段 + `/recall`（2 分钟内）+ 历史过滤 + 群广播 | ✅ done |

### 测试

20 个测试全部通过（10 个新增 + 10 个已有），覆盖率：
- 非好友消息限制、好友上限、离线好友请求
- 群组 CRUD、群聊历史
- 修改密码、黑名单
- 已读回执、消息撤回

### 新增消息类型

`MsgTypeChangePassword(400)`, `MsgTypeChangePasswordAck(401)`, `MsgTypeBlockUser(410)`, `MsgTypeUnblockUser(411)`, `MsgTypeBlacklist(412)`, `MsgTypeReadReceipt(420)`, `MsgTypeRecall(430)`, `MsgTypeRecallNotify(431)`

---

## 版本 0.8 — 2026-05-26

### Batch 1：部署与安全基础（✅ 已完成）

| 功能 | 说明 | 状态 |
|------|------|------|
| **Docker 部署** | Dockerfile 多阶段构建 + docker-compose.yml | ✅ done |
| **TLS/SSL** | wss:// 支持，自签证书脚本，客户端 wss 连接 | ✅ done |
| **限流中间件** | 连接限流 (MAX_CONNECTIONS) + 消息频率限制 (MSG_RATE_LIMIT) | ✅ done |

#### Docker 验证结果

- 镜像构建：`qqgo-server:test` 构建成功
- 容器启动：`docker compose up -d` 正常
- Health 检查：`curl http://localhost:8080/health` → `ok`
- 数据持久化：`data/qqgo.db` 在 down/up 后保留
- TLS 端到端：`wss://` 服务端 + 客户端连接成功

### Batch 2：JWT Token 认证升级（🚧 设计完成，待实现）

| 功能 | 说明 | 状态 |
|------|------|------|
| **JWT Access Token** | JWT 格式，15分钟过期，HS256 签名 | 🚧 设计完成 |
| **Refresh Token** | 随机字符串，7天过期，存 users 表 | 🚧 设计完成 |
| **旧 Token 兼容** | 直接失效，提示重新密码登录 | 🚧 设计完成 |
| **客户端自动刷新** | 401 时自动 refresh，对用户透明 | 🚧 设计完成 |

设计文档：`docs/superpowers/specs/2026-05-27-jwt-auth-design.md`

### 新增环境变量

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `TLS_CERT` | `""` | TLS 证书文件路径 |
| `TLS_KEY` | `""` | TLS 私钥文件路径 |
| `SERVER_SCHEME` | `ws` | 客户端连接协议（ws/wss） |
| `MSG_RATE_LIMIT` | `10` | 每用户每秒最大消息数 |
| `JWT_SECRET` | 随机生成 | JWT 签名密钥（Batch 2 新增） |
| `JWT_ACCESS_TTL` | `900` | Access token 过期秒数（Batch 2 新增） |
| `JWT_REFRESH_TTL_DAYS` | `7` | Refresh token 过期天数（Batch 2 新增） |

---

## 后续版本需求优先级列表

按 **紧急程度 × 重要程度 × 实现难度** 排序：

| 优先级 | 版本 | 需求 | 类型 | 说明 |
|--------|------|------|------|------|
| **P0** | v0.4 | QQ 号作为唯一标识 | refactor | ✅ 全系统 UID(string)→QQ(int64) |
| **P0** | v0.4 | 显示逻辑统一 | refactor | ✅ Remark > Nickname > QQ号 |
| **P1** | v0.4 | 发送前置校验 | bug | ✅ BUG-007：目标 QQ 不存在则拒绝发送 |
| **P1** | v0.4 | /to 前置校验 | feature | ✅ /to 时校验用户是否存在 |
| **P1** | v0.4 | 好友分组管理重做 | design | ✅ friend_groups 表，先创建再移动 |
| **P1** | v0.4 | 空分组显示 | feature | ✅ /friends 显示所有分组 |
| **P1** | v0.4 | `/login` 客户端内登录 | feature | ✅ QQ 号 + 密码登录 |
| **P1** | v0.4 | `/whoami` 用户信息 | feature | ✅ 显示 nickname / QQ |
| **P1** | v0.4 | 优雅退出 | bug | ✅ BUG-001/008：WebSocket Close Frame |
| **P1** | v0.5 | 非好友消息限制 | feature | ✅ 非好友只能发 1 条消息，删除好友后受限 |
| **P1** | v0.5 | 会话历史记录 | feature | ✅ /to 后显示最近 30 条，支持翻页 |
| **P1** | v0.5 | 群组聊天 | feature | ✅ 创建群、加群、群消息广播 |
| **P1** | v0.5 | 会话列表 | feature | ✅ `/sessions` 列出所有对话窗口 |
| **P2** | v0.6+ | Token 持久化客户端 | feature | ✅ 保存 token 到本地，免密码登录 |
| **P2** | v0.6+ | 本地聊天日志 | feature | ✅ 客户端按 QQ 号保存聊天记录到 DATA/ 目录 |
| **P1** | v0.6 | BUG-009 修复 | bugfix | ✅ 接收方消息打印顺序错乱 |
| **P1** | v0.6 | BUG-010 修复 | bugfix | ✅ /leavegroup 后无法退出群聊窗口 |
| **P1** | v0.6 | 群聊历史记录 | feature | ✅ 群聊支持 /prev /next 翻页 |
| **P1** | v0.6 | 群成员发送前校验 | feature | ✅ 客户端发消息前校验群成员身份 |
| **P2** | v0.7 | Server 优雅退出 | bugfix | ✅ srv.Shutdown + Hub 清理 + DB 关闭 |
| **P2** | v0.7 | SQLite GREATEST 兼容 | bugfix | ✅ GREATEST → MAX |
| **P2** | v0.7 | BUG-005/006 测试覆盖 | test | ✅ 好友上限 + 离线好友请求 |
| **P2** | v0.7 | 消息类型扩展 | feature | ✅ 图片、文件（base64 ≤5MB） |
| **P2** | v0.7 | 修改密码 | feature | ✅ `/changepw` + Token 刷新 |
| **P2** | v0.7 | 黑名单 | feature | ✅ `blacklists` 表 + `/block` `/unblock` |
| **P2** | v0.7 | 消息已读 | feature | ✅ `read_at` 字段 + 自动回执 |
| **P2** | v0.7 | 消息撤回 | feature | ✅ `is_recalled` + 2 分钟限制 + 历史过滤 |
| **P2** | v0.6+ | 数据库管理接口 | feature | 导出/备份/清理 |
| **P3** | v0.7+ | 桌面端 GUI | feature | Wails 或 Fyne |
| **P3** | v0.7+ | Protobuf 协议 | infra | 替换 JSON |
| **P3** | v0.8+ | TLS / 限流 / Docker | infra | 生产就绪 |
| **P3** | v0.8 | JWT Token 认证 | infra | 🚧 设计完成，待实现 |

---

## 后续开发计划

### v0.8 Batch 2：JWT Token 认证升级（当前进行中）

**状态：** 设计完成，待实现
**设计文档：** `docs/superpowers/specs/2026-05-27-jwt-auth-design.md`

**核心变更：**
- Access token (JWT, 15min, HS256) + Refresh token (随机串, 7d, 存 DB)
- 旧 SHA256 token 直接失效，不迁移
- 新增 `MsgTypeRefreshToken(107)` / `MsgTypeRefreshTokenAck(108)`
- `users` 表新增 `refresh_token` 列
- 客户端 401 自动 refresh，对用户透明

**涉及文件：**
- `internal/service/jwt.go`（新增）
- `internal/service/chat.go`（Login/LoginWithToken 签名变更）
- `internal/handler/ws.go`（handleLogin 修改，新增 handleRefreshToken）
- `internal/model/message.go`（新增消息类型和结构体）
- `internal/model/user.go`（新增 RefreshToken 字段）
- `internal/config/config.go`（新增 JWT 配置）
- `cmd/client/main.go`（启动流程、LoginAck 处理）
- `cmd/client/localstore.go`（双 token 存储）

**预估工作量：** 大

### v0.8 Batch 3：分布式基础设施（依赖 Batch 2 完成）

| 功能 | 说明 | 预估工作量 |
|------|------|-----------|
| **Redis 在线状态** | Redis 缓存在线用户（SET qq → conn_addr，TTL 心跳续期）；掉线自动过期 | 大 |
| **分布式消息路由** | NATS 或 Redis PubSub 实现跨实例消息转发 | 大 |
| **数据库管理接口** | `/backup` 导出 SQLite、`/clean` 清理过期消息（>30天） | 小 |

### v0.9：Protobuf 协议（推迟）

替换 JSON 为 Protobuf 涉及 20+ 消息类型定义、客户端/服务端双端适配、向后兼容策略。当前 JSON 协议功能完整不是瓶颈，改动量极大。

### 未规划需求

| 需求 | 优先级 | 备注 |
|------|--------|------|
| **历史消息查询** | P1 | 按时间/会话拉取历史记录 |
| **会话搜索** | P1 | 在历史记录中按关键词搜索 |
| **桌面端 GUI** | P3 | Wails 或 Fyne |
| **单元测试覆盖** | P2 | 核心模块测试覆盖提升 |