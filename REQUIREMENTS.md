# REQUIREMENTS — QQGO 需求 Backlog

---

## P0 — 核心基建

| 需求 | 说明 | 状态 | 完成版本 |
|------|------|------|----------|
| **数据库接入** | 引入 SQLite/PostgreSQL，持久化用户、好友关系、聊天记录 | ✅ done | v0.2 |
| **用户注册/登录** | 完善注册流程（密码哈希），替换当前放行模式的 Token 校验 | ✅ done | v0.2 |
| **离线消息** | 目标不在线时消息写入数据库，上线后拉取历史 | ✅ done | v0.2 |
| **消息可靠性** | ACK 确认机制 + 消息序号，保证不丢不重 | ✅ done | v0.2 |

---

## P1 — 功能完善

| 需求 | 说明 | 状态 | 完成版本 |
|------|------|------|----------|
| **好友管理** | 搜索/添加/删除好友，好友列表，分组，备注 | ✅ done | v0.3 |
| **QQ 号作为唯一标识** | 全系统 UID(string)→QQ(int64)，注册只需 nickname+password | 🚧 in progress | v0.4 |
| **显示逻辑统一** | 所有展示位置：Remark > Nickname > QQ号 | 🚧 in progress | v0.4 |
| **发送前置校验** | 只能给已存在用户发消息（BUG-007 重做） | 🚧 in progress | v0.4 |
| **好友分组管理重做** | 分组需先创建再移动（friend_groups 表，QQ 号标识） | 🚧 in progress | v0.4 |
| **客户端内登录** | `/login <qq> <password>` 命令，无需重启 client | 🚧 in progress | v0.4 |
| **用户信息查询** | `/whoami` 查看当前账号 (nickname, QQ) | 🚧 in progress | v0.4 |
| **优雅退出** | `/quit` 通过 WebSocket Close Frame 通知服务端（BUG-001/008） | 🚧 in progress | v0.4 |
| **群组聊天** | 创建群、加群、群消息广播 | 🔲 pending | v0.4 |
| **会话历史记录** | 选中会话后显示与该用户的聊天历史 | 🔲 pending | v0.4 |
| **会话列表** | 列出所有会话（每个联系人/群一个会话窗口） | 🔲 pending | v0.4 |
| **消息类型扩展** | 图片、文件、语音消息 | 🔲 pending | — |
| **历史消息查询** | 按时间/会话拉取历史记录 | 🔲 pending | — |
| **会话搜索** | 在历史记录中按关键词搜索 | 🔲 pending | — |

---

### v0.4 计划详情

#### 0. QQ 号作为唯一标识 `P0`（2026-05-22 录入）

- **现状问题：** 当前系统使用 UID(string) 作为用户唯一标识，注册需要 uid+password+nickname，不符合 QQ 设计
- **目标行为：**
  - QQ 号（QQNumber）作为全系统唯一标识
  - 注册只需 nickname + password，自动分配 QQ 号
  - 登录使用 QQ 号 + password
  - 全系统 `UID(string)` → `QQ(int64)`：User、Message(FromUID/ToUID→FromQQ/ToQQ)、Friend(UID/FriendUID→QQ/FriendQQ)、FriendGroup、Conn、Hub.conns
  - 显示逻辑统一：`Remark > Nickname > QQ号`
  - `Friend.Remark` 保留作为好友备注
  - User.Nickname 保留作为用户全局昵称
- **数据库：** 需删库重建（字段类型变更，AutoMigrate 不支持）

#### 1. 优雅退出 `P1`（BUG-001/008）

- **现状问题：** `/quit` 直接 `conn.Close()` + `os.Exit(0)`，未通过 WebSocket Close Frame 通知服务端
- **目标行为：**
  - 客户端 `/quit` 发送 WebSocket Close Frame
  - 服务端收到 Close Frame 后正常清理连接
  - 避免对端出现 `read error: use of closed network connection`

#### 2. 发送前置校验 `P1`（BUG-007 重做）

- **现状问题：** `/to` 不存在的用户后发消息，消息被存入数据库但目标永远收不到
- **目标行为：** `sendToUser` 前查 `users` 表校验目标 QQ 号存在，不存在则返回错误并拒绝存储

#### 3. 好友分组管理重做 `P1`

- **目标行为：** 需先 `/creategroup` 创建分组再移动好友，新建 `friend_groups` 表（QQ 号标识）

#### 4. 客户端内登录 `/login` `P1`

- **目标行为：** `/login <qq> <password>` 在已连接的 WebSocket 上重新登录，登录成功更新 `c.QQ`

#### 5. 用户信息查询 `/whoami` `P1`

- **目标行为：** 显示当前登录用户的信息：nickname、QQ 号

#### 6. 群组聊天 `P1`

#### 7. 会话历史记录 `P1`

#### 8. 会话列表 `P1`

#### 9. 数据库 Schema 迁移 `P1`

- **现状问题：** 每次改模型需要删除 `qqgo.db` 重新建表
- **目标：** GORM AutoMigrate 已支持新增列/表，建立迁移规范

---

## P2 — 体验与架构

| 需求 | 说明 | 状态 | 完成版本 |
|------|------|------|----------|
| **桌面端 GUI** | 引入 Wails（Go + Web 前端）或 Fyne 开发桌面客户端 | 🔲 pending | — |
| **Protobuf 协议** | 替换 JSON 序列化，降低带宽 | 🔲 pending | — |
| **分布式扩展** | 接入 NATS/Redis PubSub 实现多网关消息路由 | 🔲 pending | — |
| **在线状态** | Redis 缓存在线状态，支持状态变更通知 | 🔲 pending | — |
| **消息已读** | 已读回执、未读计数 | 🔲 pending | — |

---

## P3 — 运维与安全

| 需求 | 说明 | 状态 | 完成版本 |
|------|------|------|----------|
| **TLS/SSL** | WebSocket 加密传输 | 🔲 pending | — |
| **限流/鉴权** | 接口限流、JWT Token | 🔲 pending | — |
| **Docker 部署** | Dockerfile + docker-compose 一键启动 | 🔲 pending | — |
| **单元测试** | 核心模块测试覆盖 | 🔲 pending | — |

---

## 已知未规划需求（待讨论）

| 需求 | 说明 | 优先级 | 备注 |
|------|------|--------|------|
| **Token 持久化客户端** | 客户端保存 token，下次启动免密码登录 | P1 | — |
| **优雅退出** | `/quit` 通过 WebSocket Close Frame 通知服务端 | P1 | BUG-001, BUG-008 |
| **数据库管理接口** | 数据导出/备份/清理接口 | P2 | 测试中提出，原文不完整 |
| **修改密码** | `/changepw` 命令 | P2 | — |
| **消息撤回** | 发送方撤回已发送消息 | P2 | — |
| **黑名单** | 拉黑用户，拒绝接收消息 | P2 | — |

---

## 状态标记说明

| 标记 | 含义 |
|------|------|
| ✅ done | 已完成 |
| 🔲 pending | 待开发 |
| 🚧 in progress | 开发中 |
| ❌ cancelled | 已取消 |
