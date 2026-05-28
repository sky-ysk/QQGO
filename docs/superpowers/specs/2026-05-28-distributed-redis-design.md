# Redis 分布式消息路由 — 设计文档

> 日期：2026-05-28
> 版本：v0.8 Batch 3
> 状态：已确认

---

## 1. 背景 & 目标

### 当前状态

- 单实例部署，所有连接管理在内存中（`Hub.conns map[int64]*Conn`）
- 消息路由通过本地 `sendToUser()` 和 `broadcastToGroup()` 实现
- 用户 A 和用户 B 必须在同一实例才能实时通信
- Redis 和 NATS 配置已预留但未使用

### 目标

引入 Redis 实现在线状态管理和跨实例消息路由，支持多实例部署：

- Redis SET 存储在线用户 → 实例映射（TTL 心跳续期）
- Redis PubSub 实现跨实例消息转发
- 单实例模式保持不变（Redis 可选启用）

---

## 2. 架构概览

```
┌──────────────┐     ┌──────────────┐
│  Server A    │     │  Server B    │
│  (uuid-abc)  │     │  (uuid-xyz)  │
│              │     │              │
│  Hub.conns   │     │  Hub.conns   │
│  {10001}     │     │  {10002}     │
└──┬───────────┘     └──┬───────────┘
   │                     │
   │       Redis         │
   │  ┌──────────────────────────┐
   │  │ online:10001 → uuid-abc  │
   │  │ online:10002 → uuid-xyz  │
   │  │ ch:qq:10001  (PubSub)    │
   │  │ ch:qq:10002  (PubSub)    │
   │  │ ch:group:G123 (PubSub)   │
   │  └──────────────────────────┘

消息流：10001(A) → 10002(B)
  1. Server A 查 Redis online:10002 → uuid-xyz
  2. Server A PUBLISH ch:qq:10002
  3. Server B 订阅 ch:qq:10002，收到消息
  4. Server B 推送给本地 conns[10002]
```

---

## 3. 实例标识

- 每个实例启动时生成随机 UUID（`server-{random}`）
- 存入 Redis 用于区分消息来源，避免自己发自己收
- 重启后自动更换 UUID，旧 UUID 的消息自然丢弃

---

## 4. 在线状态管理

### 数据结构

| Key | Value | TTL | 说明 |
|-----|-------|-----|------|
| `online:{qq}` | `{instance_uuid}` | 60s | 用户当前所在实例 |

### 操作

| 事件 | 操作 |
|------|------|
| 用户上线（登录成功） | `SET online:{qq} {uuid} EX 60` |
| 心跳 | `EXPIRE online:{qq} 60`（续期） |
| 用户下线 | `DEL online:{qq}`（仅当 value 匹配当前 uuid） |

### 文件

新增 `internal/middleware/online.go`：

```go
type OnlineTracker struct {
	rdb        *redis.Client
	instanceID string
}

func NewOnlineTracker(rdb *redis.Client) *OnlineTracker
func (o *OnlineTracker) SetOnline(qq int64)
func (o *OnlineTracker) SetOffline(qq int64)
func (o *OnlineTracker) RefreshOnline(qq int64)
func (o *OnlineTracker) GetInstance(qq int64) (string, bool)
func (o *OnlineTracker) CountOnline() int
```

---

## 5. PubSub 消息路由

### 频道命名

| 频道 | 格式 | 订阅者 |
|------|------|--------|
| 私聊 | `ch:qq:{qq}` | 该用户所在实例 |
| 群聊 | `ch:group:{group_id}` | 群成员所在实例 |

### 发布消息格式

```json
{
  "source": "uuid-abc",
  "msg": { ... model.Message JSON ... }
}
```

### 文件

新增 `internal/middleware/pubsub.go`：

```go
type PubSubRouter struct {
	rdb        *redis.Client
	instanceID string
	hub        *Hub  // 回调：收到消息后推送给本地用户
}

func NewPubSubRouter(rdb *redis.Client, instanceID string, hub *Hub) *PubSubRouter
func (p *PubSubRouter) Start()                    // 启动订阅循环
func (p *PubSubRouter) Stop()                     // 停止订阅
func (p *PubSubRouter) Subscribe(qq int64)        // 用户上线时订阅 ch:qq:{qq}
func (p *PubSubRouter) Unsubscribe(qq int64)      // 用户下线时取消订阅
func (p *PubSubRouter) PublishToUser(qq int64, msg *model.Message)
func (p *PubSubRouter) PublishToGroup(groupID string, msg *model.Message)
```

### 订阅管理

- 用户上线时：`SUBSCRIBE ch:qq:{qq}`
- 用户下线时：`UNSUBSCRIBE ch:qq:{qq}`
- 群聊：群成员上线时 `SUBSCRIBE ch:group:{group_id}`，下线时 `UNSUBSCRIBE`
- 收到 PubSub 消息时：检查 `source != instanceID`（避免环路），然后查本地 `conns` 推送

---

## 6. Hub 改造

### sendToUser 改造

```
sendToUser(qq, msg):
  1. 查本地 h.conns[qq] → 有则直接推送（同实例）
  2. 本地没有 → 查 Redis online:{qq}
  3. 如果在线且 instance != self → PUBLISH ch:qq:{qq}（跨实例路由）
  4. 如果不在线 → 存 DB（现有逻辑不变）
```

### broadcastToGroup 改造

不变，内部调用 sendToUser，自动处理跨实例。

### handleHeartbeat 改造

心跳时调用 `OnlineTracker.RefreshOnline(qq)` 续期 Redis TTL。

### 用户上线/下线

- 上线（登录成功）：`SetOnline(qq)` + `Subscribe(qq)`
- 下线（RemoveUser）：`SetOffline(qq)` + `Unsubscribe(qq)`

---

## 7. 配置变更

### 环境变量

| 变量 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `REDIS_ENABLED` | bool | `false` | 是否启用 Redis 分布式模式 |

- `false`：单实例模式，不连接 Redis，走现有逻辑
- `true`：连接 Redis，启动在线状态管理 + PubSub 路由

### Config 结构体

```go
type Config struct {
	// ... existing ...
	RedisEnabled bool
}
```

`Load()` 中新增：
```go
RedisEnabled: env("REDIS_ENABLED", "false") == "true",
```

---

## 8. 依赖

| 依赖 | 用途 |
|------|------|
| `github.com/redis/go-redis/v9` | Redis 客户端 |

---

## 9. 影响范围

### 新增文件

| 文件 | 说明 |
|------|------|
| `internal/middleware/online.go` | 在线状态管理 |
| `internal/middleware/pubsub.go` | PubSub 消息路由 |
| `internal/middleware/online_test.go` | 在线状态单元测试 |
| `internal/middleware/pubsub_test.go` | PubSub 单元测试 |

### 修改文件

| 文件 | 变更 |
|------|------|
| `internal/config/config.go` | 新增 RedisEnabled |
| `cmd/server/main.go` | Redis 连接初始化、OnlineTracker/PubSubRouter 注入 Hub |
| `internal/handler/ws.go` | Hub 新增 OnlineTracker/PubSubRouter 字段，sendToUser/handleHeartbeat/RemoveUser 改造 |
| `docker-compose.yml` | 取消 Redis 注释 |
| `go.mod` / `go.sum` | 新增 go-redis 依赖 |

### 不修改文件

| 文件 | 原因 |
|------|------|
| `internal/service/chat.go` | 业务逻辑不变 |
| `internal/model/*` | 数据模型不变 |
| `cmd/client/*` | 客户端无感知 |

---

## 10. 安全考量

- Redis 不存储敏感用户数据，只存在线状态映射
- PubSub 消息不持久化，实例宕机不影响（离线消息有 DB 兜底）
- Redis 连接失败时降级为单实例模式（不报错退出）

---

## 11. 测试策略

### 单元测试

| 测试 | 验证点 |
|------|--------|
| `TestOnlineTrackerSetOnline` | SET online:{qq} 写入 |
| `TestOnlineTrackerSetOffline` | DEL online:{qq} 条件删除 |
| `TestOnlineTrackerRefresh` | EXPIRE 续期 |
| `TestOnlineTrackerGetInstance` | 查询实例 UUID |
| `TestPubSubRouterPublishSubscribe` | 发布/订阅端到端 |
| `TestPubSubRouterSourceFilter` | source != self 环路避免 |

### 集成测试

| 测试 | 验证点 |
|------|--------|
| 单实例模式（REDIS_ENABLED=false） | 不连 Redis，行为不变 |
| 多实例模式（REDIS_ENABLED=true） | 两个实例启动，跨实例消息可达 |
| 实例宕机 | 一个实例退出，另一实例正常 |

---

## 12. 后续优化（不在本批次范围）

- Redis Stream 替代 PubSub（消息持久化，实例重启不丢）
- 数据库管理接口（/backup、/clean）
- 实例健康检查（定期 PING Redis）
