# QQGO 开发状态总结

> 更新日期：2026-05-27
> 当前版本：v0.8
> 当前阶段：Batch 2（JWT Token 认证升级）设计完成，待实现

---

## 项目概况

QQGO 是基于 Go 的轻量级即时通讯系统，支持多用户终端聊天、好友管理、群组聊天、消息持久化等功能。

**技术栈：** Go 1.26 + WebSocket (gorilla/websocket) + SQLite (GORM) + JSON 序列化

**代码统计：**
- 4 packages, 27 tests（单元测试全部通过）
- 核心模块：handler (WebSocket Hub)、service (业务逻辑)、model (数据模型)、middleware (限流)

---

## 已完成版本

### v0.1 — 基础架构（2026-05-15）
WebSocket 连接管理、点对点消息、心跳保活、CLI 客户端

### v0.2 — 数据库 & 认证（2026-05-19）
SQLite 接入、bcrypt 密码哈希、离线消息、ACK 确认机制

### v0.3 — 好友系统（2026-05-20）
好友管理、搜索、分组、备注、好友请求、好友上限 500

### v0.4 — QQ 号体系（2026-05-22）
全系统 UID→QQ 号重构、客户端内登录、优雅退出、显示逻辑统一

### v0.5 — 群组 & 会话（2026-05-25）
非好友消息限制、会话历史记录、群组聊天、会话列表

### v0.6 — 客户端增强（2026-05-25）
Token 持久化、本地聊天日志、群聊历史记录、BUG 修复

### v0.7 — 功能补全（2026-05-26）
Server 优雅退出、修改密码、黑名单、图片/文件消息、消息已读/撤回

### v0.8 Batch 1 — 部署与安全（2026-05-26）✅
- Docker 部署（Dockerfile + docker-compose.yml）
- TLS/SSL（wss:// 支持）
- 限流中间件（连接限流 + 消息频率限制）
- **Docker 验证全部通过**（2026-05-27）

---

## 当前进行中的工作

### v0.8 Batch 2：JWT Token 认证升级

**状态：** 🚧 设计完成，待实现
**设计文档：** `docs/superpowers/specs/2026-05-27-jwt-auth-design.md`

#### 核心设计

| 项目 | 方案 |
|------|------|
| Access Token | JWT，15分钟过期，HS256 签名 |
| Refresh Token | 随机字符串，7天过期，存数据库 |
| 签名算法 | HS256 (HMAC-SHA256) |
| 旧 Token 兼容 | 直接失效，提示重新密码登录 |
| Refresh 触发 | 401 时自动 refresh |
| WebSocket 连接 | 建立后不检查 token 过期 |

#### 协议变更

- 新增：`MsgTypeRefreshToken(107)`, `MsgTypeRefreshTokenAck(108)`
- LoginResponse：`token` → `access_token` + `refresh_token`
- 新增：`RefreshTokenRequest` / `RefreshTokenResponse` 结构体

#### 数据库变更

- `users` 表新增 `refresh_token` 列（VARCHAR(512)）

#### 涉及文件

| 文件 | 变更类型 |
|------|---------|
| `internal/service/jwt.go` | 新增 |
| `internal/service/chat.go` | 修改（Login/LoginWithToken 签名变更，新增 RefreshToken） |
| `internal/handler/ws.go` | 修改（handleLogin 修改，新增 handleRefreshToken） |
| `internal/model/message.go` | 修改（新增消息类型和结构体） |
| `internal/model/user.go` | 修改（新增 RefreshToken 字段） |
| `internal/config/config.go` | 修改（新增 JWT 配置项） |
| `cmd/client/main.go` | 修改（启动流程、LoginAck 处理、logout） |
| `cmd/client/localstore.go` | 修改（双 token 存储） |
| `go.mod` / `go.sum` | 修改（新增 golang-jwt/jwt/v5） |

#### 预估工作量

大（改动现有 Token 系统全链路）

---

## 后续开发计划

### v0.8 Batch 3：分布式基础设施

**前置依赖：** Batch 2 完成

| 功能 | 说明 | 预估工作量 |
|------|------|-----------|
| Redis 在线状态 | Redis 缓存在线用户，TTL 心跳续期 | 大 |
| 分布式消息路由 | NATS 或 Redis PubSub 跨实例转发 | 大 |
| 数据库管理接口 | /backup 导出、/clean 清理过期消息 | 小 |

### v0.9：Protobuf 协议

替换 JSON 为 Protobuf，涉及 20+ 消息类型定义、双端适配、向后兼容策略。

**状态：** 推迟（当前 JSON 不是瓶颈，改动量极大）

### 未规划需求

| 需求 | 优先级 |
|------|--------|
| 历史消息查询（按时间/会话） | P1 |
| 会话搜索（关键词搜索） | P1 |
| 桌面端 GUI（Wails/Fyne） | P3 |
| 单元测试覆盖提升 | P2 |

---

## 版本依赖关系

```
v0.8 Batch 1 (Docker + TLS + 限流) ✅ 已完成
    ↓
v0.8 Batch 2 (JWT Token) 🚧 设计完成，待实现
    ↓
v0.8 Batch 3 (Redis + NATS + DB 管理) — 待开始
```

---

## 测试覆盖

### 当前测试状态

- **单元测试：** 27 tests，4 packages，全部通过
- **Docker 测试：** 镜像构建、compose 启动、health、数据持久化、TLS 端到端 — 全部通过

### 测试覆盖范围

- 限流中间件（ratelimit_test.go）
- WebSocket handler（ws_test.go）
- 好友管理、非好友消息限制
- 群组 CRUD、群聊历史
- 修改密码、黑名单
- 已读回执、消息撤回
- 本地存储、Token 持久化

---

## 环境信息

### 开发环境

- Go 1.26.3 darwin/arm64
- Docker (Podman backend) 5.8.2
- SQLite (GORM)

### 构建 & 测试

```bash
# 构建
go build ./...

# 测试
go test ./...

# Docker 构建
docker build -t qqgo-server:test .

# Docker 启动
docker compose up -d

# TLS 测试
bash scripts/gen-cert.sh
TLS_CERT=certs/server.crt TLS_KEY=certs/server.key go run ./cmd/server
SERVER_SCHEME=wss go run ./cmd/client
```
