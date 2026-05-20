# BUGS — QQGO 缺陷追踪

---

## v0.2 — 2026-05-19 / branch: `feature/v0.2-db-auth`

### BUG-001: `/quit` 退出时对端 read error  `open`

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

### 测试记录（原始）

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

