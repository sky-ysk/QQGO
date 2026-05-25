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
| **QQ 号作为唯一标识** | 全系统 UID(string)→QQ(int64)，注册只需 nickname+password | ✅ done | v0.4 |
| **显示逻辑统一** | 所有展示位置：Remark > Nickname > QQ号，prompt 显示 nickname | ✅ done | v0.4 |
| **发送前置校验** | 只能给已存在用户发消息（BUG-007 重做） | ✅ done | v0.4 |
| **/to 前置校验** | `/to` 时校验用户是否存在，不存在则不允许进入对话 | ✅ done | v0.4 |
| **好友分组管理重做** | 分组需先创建再移动（friend_groups 表，QQ 号标识） | ✅ done | v0.4 |
| **空分组显示** | `/friends` 显示所有分组，包括空分组 | ✅ done | v0.4 |
| **客户端内登录** | `/login <qq> <password>` 命令，无需重启 client | ✅ done | v0.4 |
| **用户信息查询** | `/whoami` 查看当前账号 (nickname, QQ) | ✅ done | v0.4 |
| **优雅退出** | `/quit` 通过 WebSocket Close Frame 通知服务端（BUG-001/008） | ✅ done | v0.4 |
| **非好友消息限制** | 非好友只能发 1 条消息，后续消息失败；删除好友后可被搜索但发消息受限 | ✅ done | v0.5 |
| **群组聊天** | 创建群、加群、群消息广播 | ✅ done | v0.5 |
| **会话历史记录** | `/to` 后显示最近 30 条历史消息，支持翻页 | ✅ done | v0.5 |
| **会话列表** | 列出所有会话（每个联系人/群一个会话窗口） | ✅ done | v0.5 |
| **消息类型扩展** | 图片、文件、语音消息 | 🔲 pending | — |
| **历史消息查询** | 按时间/会话拉取历史记录 | 🔲 pending | — |
| **会话搜索** | 在历史记录中按关键词搜索 | 🔲 pending | — |
| **本地聊天日志** | 客户端按 QQ 号分目录保存聊天记录到本地 DATA 目录 | ✅ done | v0.6 |
| **群聊历史记录** | 群聊也支持 `/prev` `/next` 翻页历史消息 | ✅ done | v0.6 |
| **群成员发送前校验** | 客户端在群聊窗口发消息前校验成员身份，非成员自动退出窗口 | ✅ done | v0.6 |
| **群聊窗口状态修复** | `/leavegroup` 后清除 targetGroupID，`/to` 清除群聊状态 | ✅ done | v0.6 |
| **接收方消息打印修复** | 修复消息打印到 prompt 中间的显示问题 | ✅ done | v0.6 |

---

### v0.4 计划详情（已完成）

#### 0. QQ 号作为唯一标识 `P0` ✅

- 全系统 `UID(string)` → `QQ(int64)`
- 注册只需 nickname + password
- 登录使用 QQ 号 + password
- 显示逻辑：Remark > Nickname > QQ号

#### 1. 优雅退出 `P1`（BUG-001/008）✅

- 客户端 `/quit` 发送 WebSocket Close Frame
- 客户端/服务端优雅处理 close 1000 (normal)

#### 2. 发送前置校验 `P1`（BUG-007 重做）✅

- `sendToUser` 前查 `users` 表校验目标 QQ 号存在

#### 3. /to 前置校验 `P1` ✅

- `/to <qq>` 时通过 `MsgTypeCheckUser` 校验用户是否存在
- 不存在则不允许进入对话

#### 4. 好友分组管理重做 `P1` ✅

- 需先 `/creategroup` 创建分组再移动好友
- 新建 `friend_groups` 表（QQ 号标识）

#### 5. 空分组显示 `P1` ✅

- `/friends` 显示所有分组，包括空分组
- FriendListResponse 增加 AllGroups 字段

#### 6. 客户端内登录 `/login` `P1` ✅

#### 7. 用户信息查询 `/whoami` `P1` ✅

- 显示 nickname + QQ 号

---

### v0.5 计划详情

#### 1. 非好友消息限制 `P1`（2026-05-22 录入）

- **现状问题：** 任何用户都可以给任何其他用户发消息并存储
- **目标行为：**
  - 好友之间可以自由发消息（无限制）
  - 非好友只能发送 **1 条**消息给对方（最后一条消息会被保存）
  - 发送第 2+ 条消息时返回错误："not friend, only 1 message allowed"
  - 删除好友后：
    - 对方仍可被搜索到
    - 给对方发消息时提示"已不是好友，只能发送 1 条消息"
    - 超过 1 条的消息发送失败
  - 重新加回好友后恢复正常
- **实现方案：**
  - 新建 `message_counts` 表：`(from_qq, to_qq, count)`，记录非好友消息计数
  - 发送消息时：检查好友关系 → 是好友则无限制 → 非好友则检查 count < 1
  - 接受好友请求时：清除双方的 message_counts 记录
  - 删除好友时：保留 message_counts（如果之前有非好友消息）

#### 2. 会话历史记录 `P1`（2026-05-22 录入）

- **目标行为：**
  - `/to <qq>` 切换会话后，自动拉取最近 30 条历史消息并展示
  - 消息按时间升序排列（从上到下），每条包含：方向标识、时间戳、内容
  - 方向标识：`[我]` 表示我发的，`[对方昵称]` 表示对方发的
  - 格式示例：
    ```
    ───── History with Alice (QQ:10001) ─────
    [Alice] 16:30:01  你好！
    [我]    16:30:15  你好，有什么事？
    [Alice] 16:31:02  想问一下...
    ─────────────────────────────────────────
    ```
  - 翻页接口：
    - `/prev` 或 `/history prev` — 向上翻 30 条（更早的消息）
    - `/next` 或 `/history next` — 向下翻 30 条（更晚的消息）
  - 服务端接口：按 `(from_qq, to_qq)` 或 `(to_qq, from_qq)` 查询，支持分页（offset + limit）
- **实现方案：**
  - 新增消息类型 `MsgTypeHistory = 312`
  - 客户端 `/to <qq>` 成功后自动发送 history 请求
  - 服务端查询 messages 表，按 created_at 排序，limit 30
  - 翻页时传递 offset 参数

#### 3. 群组聊天 `P1` ✅

- **功能：**
  - `/mkgroup <name>` 创建聊天群
  - `/joingroup <group_id>` 加入群
  - `/leavegroup <group_id>` 离开群（群主不能退出）
  - `/mygroups` 列出我的群
  - `/togroup <group_id>` 切换到群聊天
  - 群消息广播给所有成员
  - 群成员校验（非成员不能发群消息）
- **数据模型：**
  - `Group` 模型改用 `OwnerQQ int64`
  - `GroupMember` 模型改用 `QQ int64`
- **消息类型：** `MsgTypeGroupCreate(200)`, `MsgTypeGroupJoin(201)`, `MsgTypeGroupLeave(202)`, `MsgTypeGroupList(203)`, `MsgTypeGroupInfo(204)`

#### 4. 会话列表 `P1` ✅

- **功能：**
  - `/sessions` 列出所有会话窗口
  - 包含私聊会话和群聊会话
  - 显示最后一条消息内容和时间
  - 按最后消息时间倒序排列
- **消息类型：** `MsgTypeSessionList(313)`

---

### v0.6 计划详情

#### 1. BUG 修复 `P0` ✅

##### BUG-009: 接收方消息打印顺序错乱
- 客户端收到消息时，使用 `\033[2K\r` 先清除整行再打印消息
- 避免消息嵌入到 prompt 字符串中间

##### BUG-010: `/leavegroup` 后无法退出群聊窗口
- `/to` 命令执行时立即清除 `targetGroupID`
- `/leavegroup` 退出当前群时立即清除 `targetGroupID`、`targetQQ`、`historyTargetQQ`
- 确保同一时刻只有一个聊天目标（私聊或群聊）

#### 2. 群成员发送前校验 `P1` ✅

- 服务端返回 "not group member" 时，客户端自动清除 `targetGroupID` 并提示退出群聊窗口

#### 3. 群聊历史记录 `P1` ✅

- 群组聊天支持 `/prev` `/next` 翻页查看历史消息
- `/togroup <group_id>` 切换后自动拉取最近 30 条群消息
- 服务端新增 `GetGroupHistory` 接口，按 group_id 查询，支持 offset + limit 分页
- 显示格式：`[发送者昵称] 时间 内容`
- 新增消息类型 `MsgTypeGroupHistory(314)`

#### 4. 本地聊天日志 `P1` ✅

- 客户端在项目根目录创建 `DATA/` 目录
- 按 QQ 号分目录存储：`DATA/<qq_number>/`
- 私聊日志：`DATA/<qq_number>/private/<target_qq>.log`
- 群聊日志：`DATA/<qq_number>/group/<group_id>.log`
- 每条消息格式：`[时间] [方向] 内容`
- 客户端收到/发送消息时追加写入对应文件
- 新建 `cmd/client/localstore.go` 封装日志逻辑

#### 5. Token 持久化客户端 `P1` ✅

- 登录成功后将 token 保存到 `DATA/<qq_number>/token`（权限 0600）
- 客户端启动时自动扫描 DATA/ 目录，找到 token 则自动尝试 Token 登录
- `/logout` 命令清除本地 token 并退出登录状态
- Token 登录失败时自动清除过期 token，提示重新密码登录

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
