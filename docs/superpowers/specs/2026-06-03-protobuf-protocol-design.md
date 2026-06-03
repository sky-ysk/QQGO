# Protobuf 协议替换 — 设计文档

> 日期：2026-06-03
> 版本：v0.11
> 状态：已批准

---

## 1. 背景 & 目标

### 当前状态

- WebSocket 传输使用 JSON 序列化
- 消息结构：`Message{MsgType: int, Content: string}` + Content 内嵌 JSON
- 两次序列化开销：外层 Message JSON + 内层 Content JSON
- 无类型安全：Content 是 string，编译期无法检查字段

### 目标

- 替换为 Protobuf 二进制序列化，降低带宽和序列化开销
- 使用 `oneof` 替代 MsgType + Content 模式，获得编译期类型安全
- 仅替换 WebSocket 传输层，数据库层（GORM/JSON）保持不变
- 不向后兼容旧 JSON 协议，客户端和服务端同步升级

---

## 2. 架构

### 分层

```
┌─────────────────────────────────────────────┐
│  WebSocket Binary Transport                 │
│  pb.WireMessage (protobuf, oneof payload)   │
├─────────────────────────────────────────────┤
│  Handler / Client                           │
│  类型化 dispatch，无二次 JSON 解析            │
├─────────────────────────────────────────────┤
│  Service Layer (不变)                        │
│  model.Message (GORM, JSON, SQLite)         │
└─────────────────────────────────────────────┘
```

### 消息流

```
旧: Client → json.Marshal(Message) → WS Text → json.Unmarshal → dispatch by MsgType → json.Unmarshal Content
新: Client → proto.Marshal(WireMessage) → WS Binary → proto.Unmarshal → dispatch by oneof type
```

---

## 3. Proto 定义

单文件 `internal/protocol/qqgo.proto`，syntax = "proto3"。

### WireMessage 信封

```protobuf
message WireMessage {
  int64 id = 1;
  int64 client_seq = 2;
  int64 from_qq = 3;
  int64 to_qq = 4;
  string group_id = 5;
  int64 created_at = 6;  // unix timestamp

  oneof payload {
    Heartbeat heartbeat = 10;
    LoginRequest login_request = 11;
    LoginResponse login_response = 12;
    RegisterRequest register_request = 13;
    RegisterResponse register_response = 14;
    RefreshTokenRequest refresh_token_request = 15;
    RefreshTokenResponse refresh_token_response = 16;
    ServerAck server_ack = 17;
    DeliveredAck delivered_ack = 18;
    ReadReceipt read_receipt = 19;
    TextMessage text_message = 20;
    FriendRequest friend_request = 21;
    FriendAccept friend_accept = 22;
    FriendReject friend_reject = 23;
    FriendDelete friend_delete = 24;
    FriendListRequest friend_list_request = 25;
    FriendListResponse friend_list_response = 26;
    FriendSearchRequest friend_search_request = 27;
    FriendSearchResponse friend_search_response = 28;
    FriendMoveGroup friend_move_group = 29;
    FriendRemark friend_remark = 30;
    FriendGroupsRequest friend_groups_request = 31;
    FriendGroupsResponse friend_groups_response = 32;
    FriendCreateGroup friend_create_group = 33;
    FriendDeleteGroup friend_delete_group = 34;
    CheckUserRequest check_user_request = 35;
    CheckUserResponse check_user_response = 36;
    HistoryRequest history_request = 37;
    HistoryResponse history_response = 38;
    GroupHistoryRequest group_history_request = 39;
    GroupHistoryResponse group_history_response = 40;
    SearchMessagesRequest search_messages_request = 41;
    SearchMessagesResponse search_messages_response = 42;
    SessionListRequest session_list_request = 43;
    SessionListResponse session_list_response = 44;
    GroupCreateRequest group_create_request = 45;
    GroupCreateResponse group_create_response = 46;
    GroupJoinRequest group_join_request = 47;
    GroupLeaveRequest group_leave_request = 48;
    GroupListRequest group_list_request = 49;
    GroupListResponse group_list_response = 50;
    GroupInfoRequest group_info_request = 51;
    GroupInfoResponse group_info_response = 52;
    ChangePasswordRequest change_password_request = 53;
    ChangePasswordResponse change_password_response = 54;
    BlockUserRequest block_user_request = 55;
    UnblockUserRequest unblock_user_request = 56;
    BlacklistRequest blacklist_request = 57;
    BlacklistResponse blacklist_response = 58;
    RecallRequest recall_request = 59;
    RecallNotify recall_notify = 60;
    BackupRequest backup_request = 61;
    BackupResponse backup_response = 62;
    CleanRequest clean_request = 63;
    CleanResponse clean_response = 64;
    FileMessage file_message = 65;
  }
}
```

### 消息类型分组（~45 个子消息）

| 组 | 消息 |
|----|------|
| 认证 | LoginRequest, LoginResponse, RegisterRequest, RegisterResponse, RefreshTokenRequest, RefreshTokenResponse |
| 系统 | Heartbeat, ServerAck, DeliveredAck, ReadReceipt |
| 聊天 | TextMessage, FileMessage |
| 好友 | FriendRequest, FriendAccept, FriendReject, FriendDelete, FriendListRequest/Response, FriendSearchRequest/Response, FriendMoveGroup, FriendRemark, FriendGroupsRequest/Response, FriendCreateGroup, FriendDeleteGroup |
| 群组 | GroupCreateRequest/Response, GroupJoinRequest, GroupLeaveRequest, GroupListRequest/Response, GroupInfoRequest/Response |
| 历史/搜索 | HistoryRequest/Response, GroupHistoryRequest/Response, SearchMessagesRequest/Response, SessionListRequest/Response |
| 其他 | CheckUserRequest/Response, ChangePasswordRequest/Response, BlockUserRequest, UnblockUserRequest, BlacklistRequest/Response, RecallRequest, RecallNotify, BackupRequest/Response, CleanRequest/Response |

---

## 4. 改动范围

| 文件 | 变更类型 | 说明 |
|------|---------|------|
| `internal/protocol/qqgo.proto` | 新增 | proto 定义 |
| `internal/protocol/qqgo.pb.go` | 生成 | protoc 生成 |
| `pkg/websocket/conn.go` | 修改 | 支持 Binary 消息 |
| `internal/handler/ws.go` | 大改 | dispatch + 所有 handler |
| `cmd/client/main.go` | 大改 | 所有消息发送和接收 |
| `internal/model/message.go` | 精简 | 删除传输 DTO，保留 GORM 模型 |
| `go.mod` | 修改 | 新增 protobuf 依赖 |

### 不变

- `internal/service/chat.go` — 业务逻辑不变
- `internal/store/db.go` — 数据库不变
- `internal/model/user.go` — 用户模型不变
- 数据库存储格式不变

---

## 5. Handler 改造

### dispatch 方式

```go
// 旧
switch msg.MsgType {
case model.MsgTypeLogin:
    h.handleLogin(c, &msg)
}

// 新
switch p := wireMsg.Payload.(type) {
case *pb.WireMessage_LoginRequest:
    h.handleLogin(c, wireMsg, p.LoginRequest)
}
```

### handler 签名

```go
// 旧: 接收 model.Message，内部 json.Unmarshal Content
func (h *Hub) handleLogin(c *ws.Conn, msg *model.Message)

// 新: 接收类型化参数，无需二次解析
func (h *Hub) handleLogin(c *ws.Conn, wire *pb.WireMessage, req *pb.LoginRequest)
```

---

## 6. 转换层

Handler 内部需要将 protobuf 请求转换为 `model.Message`（用于数据库存储）和响应 protobuf 消息（用于发送）：

```go
// 收到聊天消息 → 存数据库
func (h *Hub) handleTextMessage(c *ws.Conn, wire *pb.WireMessage, text *pb.TextMessage) {
    msg := &model.Message{
        FromQQ:  wire.FromQq,
        ToQQ:    wire.ToQq,
        GroupID: wire.GroupId,
        Content: text.Content,
        MsgType: model.MsgTypeText,
    }
    h.svc.HandleMessage(ctx, msg)
    // ...
}
```

---

## 7. 风险

1. **改动量大** — handler 和 client 需要全部重写，约 2000+ 行代码变更
2. **protoc 工具链** — 需要安装 protoc 和 Go 插件
3. **调试难度** — 二进制消息不可直接阅读，需要工具解码
4. **WebSocket 帧类型** — 从 TextMessage 切换到 BinaryMessage

### 缓解

- 分步实施：proto 定义 → 生成代码 → handler → client
- 每步编译验证
- 保留 JSON 格式的 model.Message 用于数据库和日志
