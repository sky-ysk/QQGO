# BUGS — QQGO 缺陷追踪

---

## v0.2 — 2026-05-19 / branch: `feature/v0.2-db-auth`

### BUG-001: `/quit` 退出时对端 read error  `fixed`

**现象：**
一方先 `/quit` 退出之后，另一方退出时出现报错：

```
> [alice] > /quit
2026/05/20 10:54:04 [client] read error: read tcp [::1]:55956->[::1]:8080: use of closed network connection
```

**原因：**
客户端 `/quit` 直接 `conn.Close()` + `os.Exit(0)`，没有通过 WebSocket Close Frame 通知服务端。服务端在下一次 Read 时收到连接关闭错误，导致对端也报 read error。

**复现步骤：**
1. 启动服务端
2. 终端 A: alice 登录聊天
3. 终端 B: bob 登录聊天
4. alice: `/quit`
5. bob: `/quit` → 报错

**关联分支：** `feature/v0.2-db-auth`

---

### BUG-002: 用户注册后未自动登录，始终显示 offline  `fixed`

**现象：**
user 一直是 offline，无论怎么发消息都是不在线，发的消息全都被保存在数据库里但是用户接收不到。

**根因：**
`handleRegister` 只创建了数据库记录，但没有把用户注册到 `h.conns` 中，导致 `sendToUser` 永远查不到目标用户。

**修复：**
`handleRegister` 注册成功后自动调用登录逻辑 — 生成 token、设置 `c.UID`、加入 `h.conns`、推送离线消息。

**关联分支：** `feature/v0.2-db-auth`

---

### BUG-003: 登录/注册响应格式不一致  `fixed`

**现象：**
服务端直接 `WriteJSON(&LoginResponse{...})` 发送原始结构体，客户端期望 `Message` 包装格式（`MsgType + Content`），导致客户端 switch 无法匹配到 `MsgTypeLoginAck`。

**修复：**
将 `LoginResponse` / `RegisterResponse` 包装进 `Message{MsgType: ..., Content: json.Marshal(resp)}` 发送。

**关联分支：** `feature/v0.2-db-auth`

---

### BUG-004: 客户端 ACK 匹配失败  `fixed`

**现象：**
客户端用 `pendingMsgs[0] = text` 固定 key 0 记录待确认消息，但服务端返回的 ACK 携带真实 message ID（数据库自增），`pendingMsgs[msg.ID]` 永远匹配不到。

**修复：**
- Message 模型增加 `ClientSeq` 字段
- 服务端 ACK 回传 `ClientSeq`
- 客户端用计数器 `sentCount` 简化匹配

**关联分支：** `feature/v0.2-db-auth`

---

## v0.3 — 2026-05-20 / branch: `feature/v0.3-friend`

### BUG-005: 好友上限 500 未实际测试  `open`

**描述：**
`MaxFriends = 500` 的业务规则已编码，但未进行边界测试（接近上限、等于上限、超过上限的情况）。

**关联分支：** `feature/v0.3-friend`

---

### BUG-006: 好友离线请求通知未验证  `open`

**描述：**
当目标用户离线时，好友请求应持久化到 DB，用户上线后通过 `/friends` 可见。此流程未端到端测试。

**关联分支：** `feature/v0.3-friend`

---

### BUG-007: `/to` 不存在的用户时消息仍被存储  `fixed`

**现象：**
当 `/to nonexistent_user` 后发送消息，服务端未校验目标用户是否存在，消息被存入数据库。

**预期：**
`sendToUser` 前应先检查目标用户是否在 `users` 表中。不存在则返回错误给发送方并拒绝存储。

**复现步骤：**
1. alice 登录
2. `/to nobody`
3. 发送 "hello" → [sent ✓]，消息入库但 nobody 永远收不到

**关联分支：** `feature/v0.3-friend`

**建议：** 与需求 "先加好友才能发消息" 一起处理，统一消息发送前置校验逻辑。

---

### BUG-008: `/quit` 优雅退出缺失  `fixed`

**现象：**
用户 `/quit` 退出时，运行该客户端的终端打印：
```
[client] read error: read tcp [::1]:54379->[::1]:8080: use of closed network connection
```

**原因：**
客户端 `/quit` 直接 `conn.Close()` + `os.Exit(0)`，未通过 WebSocket Close Frame 通知服务端正常断开，服务端 ReadLoop 读到的是异常关闭错误。

**与 BUG-001 关系：** 同根因，BUG-001 侧重对端影响，BUG-008 侧重退出方自身的错误日志。

**关联分支：** `feature/v0.3-friend`

---

## v0.5 — 2026-05-25 / branch: `feature/v0.5`

### BUG-009: 接收方消息打印顺序错乱  `fixed`

**现象：**
接收方收到消息时，消息内容被打印到 prompt 中间，导致显示混乱。例如收到 "hello" 但打印的是：
```
[10002 -> 10001]: nihao:10002] >
```
消息被嵌入到 prompt 字符串中间。

**原因：**
客户端收到消息后直接 `fmt.Printf("\r...")` 输出，`\r` 只将光标移到行首，但没有清除当前行已有的内容（prompt + 用户可能已输入的文字），导致新消息与旧内容重叠。

**修复：**
所有客户端消息输出前加 `\033[2K\r`（ANSI 清除整行 + 回车），确保消息打印前清除当前行所有内容，然后再打印消息和新 prompt。

**关联分支：** `feature/v0.6`

---

### BUG-010: `/leavegroup` 后无法退出群聊窗口  `fixed`

**现象：**
在群组对话窗口中使用 `/leavegroup` 退出群后，客户端仍然停留在群聊模式。使用 `/to 10001` 切换到私聊后，虽然打印了历史消息，但发送消息时仍然被发到群组里，并且被服务端判定为非群组成员而拒绝。

**原因：**
1. `/leavegroup` 命令没有清除 `targetGroupID` 变量
2. `/to` 命令切换私聊时设置了 `targetQQ`，但没有清除 `targetGroupID`
3. 消息发送时 `GroupID` 优先于 `ToQQ`，导致消息仍发到群

**修复：**
1. `/to` 命令执行时立即清除 `targetGroupID`
2. `/leavegroup` 退出当前群时立即清除 `targetGroupID`、`targetQQ`、`historyTargetQQ`
3. 确保同一时刻只有一个聊天目标（私聊或群聊）

**关联分支：** `feature/v0.6`

---

### 测试记录（原始）

#### 2026-05-21 第一次测试（v0.3）

**通过：** 注册/登录、QQ 号分配、点对点消息+ACK、离线消息推送、好友 CRUD、搜索、备注、分组、数据持久化

**发现问题：**
- BUG-007: `/to` 不存在用户 → 消息被存储
- BUG-008: `/quit` 退出时 error log（与 BUG-001 同根因）
- Design: 好友分组应先创建再移动
- Feature: `/login` 客户端内登录、`/whoami`、发送前置校验、DB 迁移方案

#### v0.2_1
```
问题：user一直是offline，无论怎么发消息都是不在线，并且被保存在数据库里。
状态：BUGFIX → handleRegister 增加 auto-login
```

#### v0.2_2
```
TODO 1: 关于登录（token 持久化客户端侧）
TODO 2: 关于退出（优雅关闭，避免 read error）
TODO 3: /quit 退出时另一端的 read error
Feature: 好友关系 + QQ号唯一标识
```

