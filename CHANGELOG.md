# CHANGELOG — QQGO 版本变更记录

---

## [v0.8] — 2026-05-26 / branch: `main`

### Added
- **Docker 部署：** `Dockerfile`（多阶段构建）+ `docker-compose.yml`（server + SQLite volume）；Redis/NATS 预留
- **TLS/SSL：** 服务端支持 `TLS_CERT`/`TLS_KEY` 配置，有证书时启动 `wss://`，无时降级 `ws://`；自签证书生成脚本 `scripts/gen-cert.sh`
- **客户端 wss 支持：** `SERVER_SCHEME` 环境变量配置连接协议（`ws`/`wss`）；自签证书 `InsecureSkipVerify`
- **连接限流：** `MAX_CONNECTIONS` 生效，WebSocket 升级前检查连接数，超限返回 HTTP 503
- **消息频率限制：** `MSG_RATE_LIMIT` 环境变量（默认 10 条/秒），per-user 令牌桶限流，超限返回错误不断开连接
- **新依赖：** `golang.org/x/time`
- **新文件：** `Dockerfile`, `docker-compose.yml`, `.dockerignore`, `scripts/gen-cert.sh`, `internal/middleware/ratelimit.go`, `internal/middleware/ratelimit_test.go`, `internal/handler/ws_test.go`

### Changed
- **internal/config：** 新增 `TLSCert`, `TLSKey`, `MsgRateLimit` 配置项
- **cmd/server/main.go：** TLS 条件启动 + Hub 传入 maxConns 和 rateLimiter
- **cmd/client/main.go：** 连接 URL 从硬编码改为环境变量配置
- **internal/handler/ws.go：** Hub 新增 maxConns/rateLimiter 字段；ServeWS 连接限流；dispatch 消息限流；RemoveUser 清理限流器

---

## [v0.7] — 2026-05-26 / branch: `main`

### Added
- **修改密码：** `/changepw <old> <new>` 命令，旧密码校验 + bcrypt 新密码写入，自动刷新 Token
- **黑名单：** `blacklists` 表；`/block`、`/unblock`、`/blacklist` 命令；被拉黑用户发消息被拒绝
- **图片/文件消息：** `/sendimg <filepath>`、`/sendfile <filepath>` 命令；base64 编码传输（≤5MB）；接收方自动保存到 `DATA/<qq>/recv/`
- **消息已读/未读：** `messages.read_at` 字段；客户端收到消息自动发送已读回执（`MsgTypeReadReceipt`）
- **消息撤回：** `messages.is_recalled` 字段；`/recall <message_id>` 命令（发送者、2 分钟内）；撤回消息从历史中过滤；群聊撤回广播通知
- **新消息类型：** `MsgTypeChangePassword(400)`, `MsgTypeChangePasswordAck(401)`, `MsgTypeBlockUser(410)`, `MsgTypeUnblockUser(411)`, `MsgTypeBlacklist(412)`, `MsgTypeReadReceipt(420)`, `MsgTypeRecall(430)`, `MsgTypeRecallNotify(431)`
- **新数据模型：** `Blacklist` 表；`Message.ReadAt`、`Message.IsRecalled` 字段；`FileContent`、`RecallRequest`、`RecallNotify` DTO
- **新文件：** `docs/superpowers/specs/2026-05-26-v0.7-batch{1,2,3}-design.md`、`docs/superpowers/tests/2026-05-26-v0.7-test-report.md`
- **单元测试：** 10 个新测试（好友上限、离线好友请求、修改密码、黑名单、已读、撤回），总计 20 个测试

### Fixed
- **Server 优雅退出：** `srv.Close()` → `srv.Shutdown(ctx)` + `hub.Shutdown()` + `db.Close()`；Ctrl+C 后端口立即释放，可立即重启
- **SQLite GREATEST 兼容：** `LeaveGroup` 中 `GREATEST(member_cnt - 1, 0)` → `MAX(member_cnt - 1, 0)`；不再报 SQLite 函数不支持错误
- **BUG-005：** 好友上限 500 边界测试补充（TestFriendLimit500）
- **BUG-006：** 离线好友请求持久化 E2E 测试补充（TestOfflineFriendRequest）

### Changed
- **handleChatMessage：** 增加黑名单检查（被拉黑者发消息返回 "you are blocked by the recipient"）
- **GetHistoryWithTarget / GetGroupHistory：** 过滤 `is_recalled = false`，撤回消息不出现在历史中
- **客户端消息接收：** 收到文本/图片/文件消息后自动发送已读回执
- **cmd/server/main.go：** 优雅关闭流程（5s timeout + Hub 连接清理 + 数据库关闭）
- **cmd/client/localstore.go：** 新增 `saveReceivedFile` 保存接收到的文件到 `DATA/<qq>/recv/`

---

## [v0.6] — 2026-05-25 / branch: `feature/v0.6`

### Added
- **本地聊天日志：** 客户端 `DATA/` 目录按 QQ 号分目录保存私聊和群聊日志
- **Token 持久化：** 登录成功自动保存 token，客户端启动自动 Token 登录，`/logout` 清除
- **群聊历史记录：** `/togroup` 后自动拉取 30 条群消息，`/prev` `/next` 翻页
- **新消息类型：** `MsgTypeGroupHistory(314)`
- **新文件：** `cmd/client/localstore.go`（日志 + Token 管理）
- **单元测试：** 20 个测试覆盖本地存储、Token、群聊历史

### Fixed
- **BUG-009：** 接收方消息打印到 prompt 中间（`\033[2K` 清除整行）
- **BUG-010：** `/leavegroup` 后无法退出群聊窗口（清除 targetGroupID）
- **群成员校验：** 服务端返回 "not group member" 时自动退出群聊窗口

### Changed
- **客户端启动：** 自动扫描 DATA/ 目录尝试 Token 登录
- **/to 命令：** 切换私聊时自动清除 targetGroupID 和群历史上下文
- **/leavegroup：** 退出当前群时清除所有群相关状态
- **消息收发：** 发送和接收消息时自动追加本地日志

---

## [v0.5] — 2026-05-25 / branch: `feature/v0.5`

### Added
- **非好友消息限制：** 非好友只能发 1 条消息，好友无限制；`message_counts` 表记录计数；接受好友时清除计数
- **会话历史记录：** `/to` 后自动拉取最近 30 条历史消息；`/prev` `/next` 翻页；`[我]` / `[对方昵称]` 方向标识
- **群组聊天：** `/mkgroup` 创建群、`/joingroup` 加群、`/leavegroup` 退群、`/mygroups` 群列表、`/togroup` 切换群聊；群消息广播；群成员校验
- **会话列表：** `/sessions` 列出所有私聊和群聊会话，显示最后一条消息和时间
- **新消息类型：** `MsgTypeHistory(312)`, `MsgTypeSessionList(313)`, `MsgTypeGroupList(203)`, `MsgTypeGroupInfo(204)`
- **新数据模型：** `MessageCount` 表（非好友消息计数）、`HistoryMessage`/`HistoryResponse`/`SessionInfo` DTO
- **单元测试：** 5 个测试覆盖非好友限制、历史记录、群组功能

### Changed
- **Group/GroupMember 模型：** `OwnerUID(string)` → `OwnerQQ(int64)`，`UID(string)` → `QQ(int64)`
- **GetGroupMembers：** 从 `[]string` 改为 `[]int64`，真实数据库查询
- **handleChatMessage：** 增加非好友消息限制检查和群成员校验
- **broadcastToGroup：** 从内存 `h.groups` 改为查询数据库
- **客户端 prompt：** 支持显示群聊目标 `Group:xxx`

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
