# 下一步开发计划

**当前版本：** v0.12（测试覆盖率提升已完成）
**更新日期：** 2026-06-03

---

## 待完成需求（按优先级排序）

| 优先级 | 需求 | 说明 | 预估工作量 |
|--------|------|------|-----------|
| P3 | **桌面端 GUI** | Wails 或 Fyne 桌面客户端 | 大 |

> **已完成：** 分布式扩展（Redis PubSub）✅、在线状态（Redis 缓存）✅、数据库管理接口 ✅、JWT 双 token ✅、Protobuf 协议 ✅、单元测试覆盖 ✅

---

## v0.12 测试覆盖率提升（2026-06-03）

### 覆盖率变化

| 包 | 之前 | 之后 | 提升 |
|---|------|------|------|
| `internal/service` | 60.8% | **90.4%** | +29.6% |
| `internal/middleware` | 41.5% | **98.9%** | +57.4% |
| `internal/handler` | 1.2% | **74.9%** | +73.7% |
| `cmd/client` | 集成测试 8/8 | 集成测试 8/8 | 不变 |

### 关键技术决策

1. **FTS5 支持**：`mattn/go-sqlite3` 内置 FTS5，只需加 build tag `-tags fts5`，无需 Docker
2. **Redis 测试**：使用 `miniredis`（内存 Redis mock），比 Docker 更快更稳定
3. **Handler 测试**：使用 `httptest.NewServer` + gorilla websocket 客户端，覆盖 40+ 个 handler 函数

### 新增测试文件

- `internal/service/chat_test.go`：+25 个测试（ValidateToken、GetOfflineMessages、MarkDelivered、GetHistory、RejectFriend、SearchUsers、MoveFriendGroup、GetFriendGroups、SetRemark、CreateFriendGroup、DeleteFriendGroup、BackupDB 等）
- `internal/service/jwt_test.go`：+2 个测试（InitJWT 空密钥、GetRefreshTTLDays）
- `internal/middleware/online_test.go`：+7 个测试（Set/Get/Refresh/Count/多实例隔离）
- `internal/middleware/pubsub_test.go`：+7 个测试（真实发布/订阅、自身消息跳过、非法 JSON/Protobuf）
- `internal/handler/ws_test.go`：+40 个测试（好友/群组/历史/搜索/密码/黑名单/文件/撤回/备份/清理/未登录拦截）

### 运行测试

```bash
# 完整测试（推荐）
go test -tags fts5 ./internal/... ./cmd/client/

# 带覆盖率
go test -tags fts5 -cover ./internal/...

# 仅 handler
go test -v ./internal/handler/
```

### 剩余未覆盖（~25% handler）

主要是 `sendToUser`/`broadcastToGroup`（需要多连接并发场景）、`pushOfflineMessages`（需要离线消息推送时序）、`notifyFriendRequest/Accepted`（需要双客户端在线通知）——这些更适合集成测试而非单元测试。

---

## 建议：下一步开发

### 桌面端 GUI（P3）

1. **技术选型** — Wails（Go + Web 前端）vs Fyne（纯 Go GUI）
2. **核心功能** — 登录/注册、好友列表、私聊/群聊、文件传输
3. **与 CLI 客户端共存** — 共享 `internal/` 层，独立 `cmd/desktop/` 入口

---

## 下次对话提示词

### 通用提示词（推荐）

```
继续开发 QQGO 项目。

项目路径：/Users/yangshikang.6/Desktop/Code/Go/QQGO
当前版本：v0.12（测试覆盖率提升已完成）

请先阅读以下文件了解项目状态：
- docs/superpowers/plans/next-steps.md（本文件）
- REQUIREMENTS.md（需求清单）
- CHANGELOG.md（版本历史）
- CLAUDE.md（开发规范）

当前覆盖率：
- internal/service：90.4%
- internal/middleware：98.9%
- internal/handler：74.9%

按照 next-steps.md 的建议，下一步是开发桌面端 GUI（P3）。

技术选型建议：
- Wails：Go 后端 + Web 前端（HTML/CSS/JS），生态成熟，UI 灵活
- Fyne：纯 Go GUI，跨平台原生，但 UI 定制性较低

请开始桌面端 GUI 的技术选型和设计。
```

### 直接开始桌面端 GUI

```
继续开发 QQGO 桌面端 GUI。

项目路径：/Users/yangshikang.6/Desktop/Code/Go/QQGO

当前架构：
- 服务端：cmd/server/main.go（WebSocket + Protobuf）
- CLI 客户端：cmd/client/main.go（终端交互）
- 共享层：internal/（handler、service、model、protocol、middleware）
- 协议：Protobuf 二进制传输（internal/protocol/qqgo.proto）

桌面端需求：
1. 技术选型：Wails（推荐）或 Fyne
2. 核心功能：登录/注册、好友列表、私聊/群聊、文件传输、历史记录
3. 架构：共享 internal/ 层，新建 cmd/desktop/ 入口
4. 与 CLI 客户端共存

请先进行技术选型分析，然后设计桌面端架构。
```

---

## 项目架构概览

### 目录结构

```
QQGO/
├── cmd/
│   ├── server/main.go          # 服务端入口
│   └── client/main.go          # CLI 客户端入口
├── internal/
│   ├── handler/ws.go           # WebSocket 消息分发和处理（Protobuf dispatch）
│   ├── service/chat.go         # 业务逻辑（用户、好友、群组、消息）
│   ├── service/jwt.go          # JWT 双 token 认证
│   ├── model/                  # 数据模型（User、Message、Friend、Group 等）
│   ├── protocol/qqgo.proto     # Protobuf 协议定义（45+ 消息类型）
│   ├── middleware/             # 限流、在线状态、PubSub
│   ├── config/                 # 配置加载
│   └── store/db.go             # SQLite + GORM 初始化
├── pkg/websocket/conn.go       # WebSocket 连接封装（Binary 模式）
├── docs/superpowers/           # 设计文档和计划
├── DATA/                       # 客户端本地数据（聊天记录、token）
└── qqgo.db                     # 服务端 SQLite 数据库
```

### 关键技术栈

- **语言**：Go 1.26.3
- **数据库**：SQLite + GORM
- **协议**：WebSocket（gorilla/websocket）+ Protobuf（google.golang.org/protobuf）
- **认证**：JWT 双 token（Access 15min + Refresh 7d）
- **分布式**：Redis PubSub（可选）
- **部署**：Docker + docker-compose

### 消息协议

- **传输格式**：Protobuf 二进制（`internal/protocol/qqgo.pb.go`）
- **信封结构**：`WireMessage{Id, ClientSeq, FromQq, ToQq, GroupId, CreatedAt, oneof Payload}`
- **消息类型**：45+ 种（认证、好友、群组、聊天、历史、搜索、管理等）
- **分发机制**：handler 使用 `switch p := wire.Payload.(type)` 类型匹配

### 开发规范

- **CLAUDE.md**：编码规范（先思考再编码、最简实现、外科手术式修改、目标驱动）
- **测试优先**：新功能先写测试，再实现
- **覆盖率目标**：service 层 > 70%，handler 层由集成测试覆盖
- **提交规范**：`feat/fix/docs/test(scope): description`

---

## 已完成功能清单

### v0.1-v0.9（核心功能）
- WebSocket 多用户连接管理
- 用户注册/登录（bcrypt + JWT）
- 离线消息持久化 + 上线推送
- ACK 确认机制
- 好友管理（添加/删除/分组/备注）
- QQ 号作为唯一标识
- 群组聊天（创建/加入/退出/广播）
- 会话历史记录 + 翻页
- 会话列表
- 消息类型扩展（图片/文件/语音）
- 历史消息查询（时间范围）
- 全文搜索（FTS5）
- 本地聊天日志
- Token 持久化
- 非好友消息限制
- 消息已读/撤回
- 黑名单
- 修改密码

### v0.10（数据库管理 + JWT）
- `/backup` 导出 SQLite 数据库
- `/clean <days>` 清理过期消息
- JWT 双 token 认证（Access + Refresh）
- 客户端自动刷新 token

### v0.11（Protobuf 协议）
- 全量替换 JSON 为 Protobuf 二进制传输
- 45+ 消息类型定义在 `.proto` 文件
- 类型安全 dispatch（oneof 替代 MsgType 整数）
- Binary WebSocket 模式
- PubSub 中间件改用 Protobuf
- 单元测试覆盖率提升至 60.8%

---

## 已知问题和限制

1. **FTS5 依赖**：SQLite FTS5 模块在某些环境不可用，搜索功能会降级
2. **单实例部署**：Redis PubSub 可选，默认单实例运行
3. **无 GUI**：目前只有 CLI 客户端，桌面端待开发
4. **测试覆盖率**：handler 层覆盖率低（需要 WebSocket 环境），由集成测试补充
5. **文件传输**：base64 编码，最大 5MB，大文件需要优化

---

## 下一步开发建议

### 短期（v0.12）
- **桌面端 GUI**：技术选型（Wails vs Fyne）+ 核心功能实现
- **测试覆盖率**：继续提升 service 层至 70%+

### 中期（v0.13-v0.14）
- **性能优化**：大文件分片传输、消息队列异步处理
- **安全加固**：消息加密、防重放攻击、IP 黑名单

### 长期（v1.0+）
- **多端同步**：移动端（iOS/Android）
- **插件系统**：机器人、自动回复、消息转发
- **云存储**：文件上传到 OSS/S3，数据库迁移到 PostgreSQL
