# CHANGELOG — QQGO 版本变更记录

---

## [v0.4] — 2026-05-22 / branch: `feature/v0.4-qq-identity`

### Changed
- **全系统重构：** QQ 号作为唯一标识（UID(string) → QQ(int64)）
- **注册流程：** 只需 `nickname + password`，自动分配 QQ 号
- **登录流程：** 使用 QQ 号 + 密码登录
- **数据模型：** User 移除 UID 字段，Message(FromQQ/ToQQ)、Friend(QQ/FriendQQ)、FriendGroup(QQ) 全部使用 QQ 号
- **连接管理：** Hub.conns 使用 `map[int64]*Conn`，Conn.QQ 替代 Conn.UID
- **客户端：** 启动无需参数，使用 `/login` 或 `/register` 手动登录
- **显示逻辑：** 好友列表/搜索结果使用 `Remark > Nickname`，不再显示 UID

---

## [v0.3] — 2026-05-20 / branch: `feature/v0.3-friend`

### Added
- QQ 号系统（注册时自动分配，QQNumber = 10000 + ID，起始 10001）
- 好友添加/接受/拒绝/删除（`/addfriend`, `/accept`, `/reject`, `/delfriend`）
- 好友列表分组显示（`/friends`）
- 用户搜索（`/search <keyword>`，支持 QQ 号精确 + 昵称模糊）
- 好友分组管理（`/movefriend <qq> <group>`, `/groups`）
- 好友备注（`/remark <qq> <remark>`）
- 好友请求实时在线通知（目标在线时即时推送）
- 好友上限 500（`MaxFriends` 常量）
- 8 个新消息类型：`MsgTypeFriendReject`(302) ~ `MsgTypeFriendGroups`(308)
- Register 返回值增加 `QQNumber`
- Login 返回值增加 `QQNumber`
- Friend 模型增加 `Status`（0=pending, 1=accepted, 2=rejected）、`GroupName`

### Changed
- `dispatch` 路由增加 300-308 case，不再 fallback 当聊天消息处理
- `Service` 接口扩展 9 个好友方法

---

## [v0.2] — 2026-05-19 / branch: `feature/v0.2-db-auth`

### Added
- SQLite + GORM 数据库接入（`store/db.go`，AutoMigrate 5 张表）
- bcrypt 密码哈希用户注册/登录
- 离线消息持久化 + 上线自动推送（`pushOfflineMessages`）
- ACK 消息确认机制（`MsgTypeServerAck` 发送确认 + `MsgTypeDelivered` 送达确认）
- 环境变量 `DB_PATH`（默认 `./qqgo.db`）、`BCRYPT_COST`
- 客户端 `/register` 命令

### Changed
- Token 校验从放行模式（`return true, nil`）改为真实数据库校验
- `ChatService` 从内存切片存储改为 GORM
- `Message` 模型增加 `Delivered`、`ClientSeq` 字段
- User 模型增加 `PasswordHash`、`Token` 字段

### Fixed
- 注册后未自动登录导致用户始终 offline（BUG-002）
- 登录/注册响应格式不一致（BUG-003）
- ACK ClientSeq 匹配失败（BUG-004）

---

## [v0.1] — 2026-05-15 / branch: `main`

### Added
- WebSocket 多用户连接管理（Hub + Conn 封装）
- 点对点文本消息实时转发
- CLI 客户端交互式聊天（`/to`, `/who`, `/help`, `/quit`）
- 心跳保活（服务端 Ping 30s / 客户端 Pong 60s）
- 内存消息队列（10 万条上限，超出截断一半）
- 登录认证流程（Token 放行模式）
- 环境变量配置（`SERVER_HOST`, `SERVER_PORT`, `MAX_CONNECTIONS`）
- 优雅关闭（SIGINT/SIGTERM）
