# JWT Token 认证升级 — 设计文档

> 日期：2026-05-27
> 版本：v0.8 Batch 2
> 状态：待评审

---

## 1. 背景 & 目标

### 当前状态

- Token 格式：SHA256(32字节随机) = 64字符十六进制字符串
- 存储：`users.token` 列（VARCHAR(128)），每次密码登录覆盖
- 验证：数据库查询 `WHERE qq_number = ? AND token = ?` 字符串匹配
- 过期：**无过期机制**，token 永久有效
- 客户端：token 保存在 `DATA/<qq>/token` 文件，权限 0600

### 目标

替换为 JWT 认证体系，引入 access token + refresh token 双 token 机制：

- Access token：JWT 格式，15分钟过期，用于 WebSocket 连接认证
- Refresh token：随机字符串，7天过期，存数据库，用于换取新 access token
- 旧 SHA256 token 直接失效，不迁移，登录失败时提示重新密码登录

---

## 2. 架构概览

```
┌─────────────┐         ┌──────────────────┐         ┌──────────────┐
│   Client    │◄───────►│  Server (ws.go)  │◄───────►│ ChatService  │
│  (CLI)      │  WS     │  handleLogin     │         │ Login()      │
│             │         │  handleRefresh   │         │ RefreshToken()│
│ - 保存双    │         │                  │         │ LoginWith    │
│   token     │         │                  │         │   Token()    │
│ - 401 自动  │         │                  │         │              │
│   refresh   │         │                  │         │              │
└─────────────┘         └──────────────────┘         └──────────────┘
                                                          │
                                                    ┌─────▼─────┐
                                                    │   SQLite   │
                                                    │ users 表   │
                                                    │ refresh_   │
                                                    │ token 列   │
                                                    └───────────┘
```

### Token 生命周期

```
密码登录 → 返回 {access_token (15min), refresh_token (7d)}
                access_token 用于 WS 连接
                refresh_token 存 users.refresh_token 列

Access token 过期 → 客户端发 refresh 请求（通过 WS 连接，无需先登录）
    → 服务端验证 refresh_token → 返回新 access_token
    → 客户端保存新 access_token → 用新 token 重新发起登录

Refresh token 过期 → 返回 401 → 客户端清除本地 token → 提示密码登录

密码登录/logout → 清除 users.refresh_token → 旧 refresh_token 失效
```

### 关键设计决策

| 决策 | 选择 | 理由 |
|------|------|------|
| 签名算法 | HS256 (HMAC-SHA256) | 单实例部署，对称加密最简单高效 |
| 旧 token 兼容 | 直接失效，提示密码登录 | 不强制迁移，最简洁 |
| WebSocket 连接 | 建立后不检查 token 过期 | 长连接特性，避免中断聊天 |
| Refresh 触发 | 401 时自动 refresh | 对用户透明，体验最佳 |
| Refresh token 存储 | 存 users 表 | 支持服务端撤销（logout/密码登录时覆盖） |

---

## 3. 数据库变更

### users 表新增字段

```go
type User struct {
    // ... existing fields ...
    RefreshToken string `gorm:"size:512" json:"-"`  // 新增：refresh token 存储
}
```

- 字段名：`refresh_token`
- 类型：VARCHAR(512)
- 可为空：是
- 索引：不需要（查询通过 qq_number + refresh_token 组合）

### 迁移方式

GORM AutoMigrate 自动添加列，无需手动 SQL 迁移。

---

## 4. 配置变更

### 环境变量（`internal/config/config.go`）

| 变量 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `JWT_SECRET` | string | 随机生成 32 字节 | JWT 签名密钥 |
| `JWT_ACCESS_TTL` | int | 900 | Access token 过期时间（秒），默认 15 分钟 |
| `JWT_REFRESH_TTL_DAYS` | int | 7 | Refresh token 过期时间（天） |

### JWT_SECRET 行为

- 未配置时：启动时随机生成 32 字节密钥（每次重启不同，所有已签发 JWT 失效）
- 已配置时：使用配置的密钥（重启后 JWT 仍有效）
- 生产环境**必须**配置此变量

---

## 5. 协议变更

### 5.1 新增消息类型

```go
const (
    MsgTypeRefreshToken    MessageType = 107  // 客户端请求刷新 access token
    MsgTypeRefreshTokenAck MessageType = 108  // 服务端返回新 access token
)
```

### 5.2 LoginResponse 变更

```go
// 旧版
type LoginResponse struct {
    Code     int    `json:"code"`
    Message  string `json:"message"`
    Token    string `json:"token,omitempty"`      // 单一 token
    Online   int    `json:"online"`
    QQNumber int64  `json:"qq_number,omitempty"`
    Nickname string `json:"nickname,omitempty"`
}

// 新版
type LoginResponse struct {
    Code         int    `json:"code"`
    Message      string `json:"message"`
    AccessToken  string `json:"access_token,omitempty"`   // JWT, 15min
    RefreshToken string `json:"refresh_token,omitempty"`  // 随机串, 7d
    Online       int    `json:"online"`
    QQNumber     int64  `json:"qq_number,omitempty"`
    Nickname     string `json:"nickname,omitempty"`
}
```

**向后兼容**：旧客户端收到 `access_token`/`refresh_token` 字段会忽略（JSON 反序列化不报错），但 `token` 字段为空会导致旧客户端行为异常。这是预期行为 — 旧 token 已失效，旧客户端需升级。

### 5.3 RefreshTokenRequest / RefreshTokenResponse

```go
type RefreshTokenRequest struct {
    QQ           int64  `json:"qq"`
    RefreshToken string `json:"refresh_token"`
}

type RefreshTokenResponse struct {
    Code        int    `json:"code"`
    Message     string `json:"message"`
    AccessToken string `json:"access_token,omitempty"`
}
```

### 5.4 错误码约定

| Code | Message | 场景 | 客户端行为 |
|------|---------|------|-----------|
| 401 | `token expired` | Access token 过期 | 尝试 refresh |
| 401 | `refresh token expired` | Refresh token 过期 | 清除 token，提示密码登录 |
| 401 | `auth failed` | 旧 SHA256 token 或无效 token | 清除 token，提示密码登录 |

---

## 6. 服务端改造

### 6.1 JWT 工具层（新增 `internal/service/jwt.go`）

```go
package service

// GenerateAccessToken 签发 JWT access token
// Claims: {qq: int64, exp: unix_timestamp}
func GenerateAccessToken(qq int64) (string, error)

// ValidateAccessToken 验证 JWT access token，返回 qq 号
// 错误类型：ErrTokenExpired（JWT 过期）、ErrInvalidToken（无效 token/旧 SHA256 token）
func ValidateAccessToken(tokenString string) (int64, error)

// GenerateRefreshToken 生成随机 refresh token
func GenerateRefreshToken() string
```

**JWT Claims 结构**：

```json
{
    "qq": 10001,
    "exp": 1717000000
}
```

- `qq`：用户 QQ 号
- `exp`：过期时间戳（标准 JWT claim）
- 不包含其他信息（nickname 等），保持最小化

### 6.2 ChatService 方法变更

#### `Login` — 签名变更

```go
// 旧
func (s *ChatService) Login(qq int64, password string) (string, error)

// 新
func (s *ChatService) Login(qq int64, password string) (accessToken, refreshToken string, err error)
```

**行为变更**：
1. 验证密码（不变）
2. 生成 JWT access token（新）
3. 生成随机 refresh token，存入 `users.refresh_token`（新）
4. 清除旧的 `users.token` 字段（新，清理旧数据）
5. 返回双 token

#### `LoginWithToken` — 逻辑变更

```go
// 旧：查询数据库匹配 token
func (s *ChatService) LoginWithToken(qq int64, token string) (bool, error)

// 新：验证 JWT，不查数据库
func (s *ChatService) LoginWithToken(qq int64, token string) (bool, error)
```

**行为变更**：
1. 调用 `ValidateAccessToken(token)` 解析 JWT
2. 验证解析出的 QQ 号与传入的 qq 匹配
3. 不查询数据库（性能提升）
4. 旧 SHA256 token 解析失败 → 返回 false → 客户端收到 401 "auth failed"

**错误类型**：
- `ErrTokenExpired`：JWT 已过期 → handler 发送 "token expired"
- `ErrInvalidToken`：无效 token（旧 SHA256 token、格式错误、签名不匹配等）→ handler 发送 "auth failed"

#### `RefreshToken` — 新增方法

```go
func (s *ChatService) RefreshToken(qq int64, refreshToken string) (newAccessToken string, err error)
```

**行为**：
1. 查询 `users` 表：`WHERE qq_number = ? AND refresh_token = ?`
2. 检查 refresh token 是否过期（`updated_at + 7天 < now`）
3. 验证通过 → 生成新 JWT access token → 返回
4. 验证失败 → 返回 error

#### `Logout` — 新增方法（服务端侧）

```go
func (s *ChatService) ClearRefreshToken(qq int64) error
```

**行为**：清除 `users.refresh_token` 字段。

#### `generateToken` — 删除

旧的 `generateToken()` 函数删除，替换为 `GenerateAccessToken` + `GenerateRefreshToken`。

### 6.3 Handler 变更（`internal/handler/ws.go`）

#### `handleLogin` 修改

密码登录分支：
```go
// 旧
token, err := h.svc.Login(req.QQ, req.Password)
// 发送 LoginAck with token

// 新
accessToken, refreshToken, err := h.svc.Login(req.QQ, req.Password)
// 发送 LoginAck with access_token + refresh_token
```

Token 登录分支：
```go
// 旧
ok, err := h.svc.LoginWithToken(req.QQ, req.Token)

// 新（接口不变，内部逻辑改为 JWT 验证）
ok, err := h.svc.LoginWithToken(req.QQ, req.Token)
// 失败时区分错误类型：
//   - JWT 过期 → "token expired"
//   - 旧 token/无效 → "auth failed"
```

#### `handleRefreshToken` — 新增

```go
func (h *Hub) handleRefreshToken(c *Connection, msg *model.Message) {
    // 1. 解析 RefreshTokenRequest
    // 2. 调用 h.svc.RefreshToken(qq, refreshToken)
    // 3. 成功 → 发送 RefreshTokenAck with new access_token
    // 4. 失败 → 发送 RefreshTokenAck with code 401
}
```

**重要**：`handleRefreshToken` 必须在**未认证的 WS 连接**上也能工作。当 access token 过期导致 token 登录失败时，客户端尚未完成认证，但 WS TCP 连接已建立。Refresh 请求通过此未认证连接发送，服务端从请求 payload 中获取 QQ 号（而非从连接的认证状态）。

### 6.4 Service 接口变更

```go
type Service interface {
    // 签名变更
    Login(qq int64, password string) (accessToken, refreshToken string, err error)
    // 逻辑变更（JWT 验证，不查 DB）
    LoginWithToken(qq int64, token string) (bool, error)
    // 新增
    RefreshToken(qq int64, refreshToken string) (newAccessToken string, err error)
    ClearRefreshToken(qq int64) error
    // ... 其他方法不变
}
```

---

## 7. 客户端改造

### 7.1 Token 存储变更

```
旧: DATA/<qq>/token                    (单一 token 文件)
新: DATA/<qq>/access_token             (JWT, 15min)
    DATA/<qq>/refresh_token            (随机串, 7d)
```

`cmd/client/localstore.go` 变更：

```go
// 新增
func saveAccessToken(qq int64, token string)
func loadAccessToken(qq int64) (string, bool)
func saveRefreshToken(qq int64, token string)
func loadRefreshToken(qq int64) (string, bool)

// 修改
func removeToken(qq int64)  // 同时清除 access_token + refresh_token

// 保留（向后兼容）
func saveToken(qq int64, token string)   // 废弃，不再调用
func loadToken(qq int64) (string, bool)  // 废弃，不再调用
```

### 7.2 启动流程变更

```
启动 → 扫描 DATA/ 目录
  → 找到 access_token → 用 access_token 登录 (MsgTypeLogin)
    → 成功 → 正常进入聊天
    → 401 "token expired" → 检查 refresh_token
      → 有 refresh_token → 发 MsgTypeRefreshToken
        → 成功 → 保存新 access_token → 重新登录
        → 失败 → 清除双 token → 提示密码登录
      → 无 refresh_token → 提示密码登录
    → 401 "auth failed" → 清除旧 token → 提示密码登录
  → 无 access_token → 检查旧 token（兼容）
    → 有旧 token → 尝试登录 → 必失败 → 清除 → 提示密码登录
  → 无任何 token → 提示密码登录
```

### 7.3 401 自动刷新

在 `handleLoginAck` 中：

```go
case model.MsgTypeLoginAck:
    if resp.Code == 401 {
        if resp.Message == "token expired" {
            // access token 过期，尝试 refresh
            if refreshToken := loadRefreshToken(myQQNumber); refreshToken != "" {
                sendRefreshRequest(conn, myQQNumber, refreshToken)
                return
            }
        }
        // refresh token 也过期了，或旧 token 无效
        clearTokens(myQQNumber)
        promptPasswordLogin()
    }
```

### 7.4 Refresh 请求处理

```go
func sendRefreshRequest(conn *websocket.Conn, qq int64, refreshToken string) {
    payload := model.RefreshTokenRequest{
        QQ:           qq,
        RefreshToken: refreshToken,
    }
    // 发送 MsgTypeRefreshToken
}

func handleRefreshTokenAck(resp *model.RefreshTokenResponse) {
    if resp.Code == 0 {
        saveAccessToken(myQQNumber, resp.AccessToken)
        // 用新 access token 重新登录
        loginWithAccessToken(conn, myQQNumber, resp.AccessToken)
    } else {
        clearTokens(myQQNumber)
        promptPasswordLogin()
    }
}
```

### 7.5 /logout 变更

```
旧: 清除本地 token 文件
新: 清除本地 access_token + refresh_token 文件
    （服务端不需要通知，refresh_token 在 DB 中，下次密码登录自动覆盖）
```

### 7.6 /changepw 变更

```
修改密码成功 → 服务端返回新 access_token + refresh_token
            → 客户端保存新双 token（覆盖旧文件）
```

---

## 8. 安全考量

### JWT 密钥管理

- `JWT_SECRET` 通过环境变量配置，不硬编码
- 未配置时随机生成（开发模式），重启后所有 JWT 失效
- 生产环境必须配置固定密钥

### Token 泄露防护

- Access token 短期有效（15分钟），泄露窗口小
- Refresh token 存数据库，可随时撤销
- 密码登录/修改密码时自动覆盖 refresh token
- 客户端 token 文件权限 0600（不变）

### 旧 Token 清理

- 旧 SHA256 token 在 `users.token` 列中，密码登录时清除
- 可考虑在 AutoMigrate 后添加清理逻辑：`UPDATE users SET token = ''`

---

## 9. 测试策略

### 服务端单元测试

| 测试 | 验证点 |
|------|--------|
| `TestGenerateAccessToken` | JWT 签发、claims 解析 |
| `TestValidateAccessToken` | 有效 token、过期 token、无效 token |
| `TestLogin` | 返回双 token、密码错误 |
| `TestLoginWithToken` | JWT 验证、旧 token 失效 |
| `TestRefreshToken` | 有效 refresh、过期 refresh、无效 refresh |

### 集成测试

| 测试 | 验证点 |
|------|--------|
| 密码登录 → 获取双 token | LoginAck 包含 access_token + refresh_token |
| Access token 登录 | 正常连接 |
| Access token 过期 → refresh | 自动获取新 access token |
| Refresh token 过期 | 提示密码登录 |
| 旧 SHA256 token 登录 | 返回 401 "auth failed" |
| 修改密码 → token 刷新 | 新双 token 生效 |

---

## 10. 影响范围

### 修改文件

| 文件 | 变更类型 | 说明 |
|------|---------|------|
| `internal/model/user.go` | 修改 | 新增 RefreshToken 字段 |
| `internal/model/message.go` | 修改 | 新增消息类型、修改 LoginResponse、新增 RefreshToken 结构体 |
| `internal/config/config.go` | 修改 | 新增 JWT 配置项 |
| `internal/service/jwt.go` | **新增** | JWT 签发/验证工具 |
| `internal/service/chat.go` | 修改 | Login/LoginWithToken 签名变更，新增 RefreshToken/ClearRefreshToken |
| `internal/handler/ws.go` | 修改 | handleLogin 修改，新增 handleRefreshToken |
| `cmd/client/main.go` | 修改 | 启动流程、LoginAck 处理、logout、changepw |
| `cmd/client/localstore.go` | 修改 | 新增双 token 存储函数 |
| `go.mod` / `go.sum` | 修改 | 新增 JWT 依赖（`golang-jwt/jwt/v5`） |

### 不修改文件

| 文件 | 原因 |
|------|------|
| `internal/store/db.go` | AutoMigrate 自动处理 schema 变更 |
| `Dockerfile` / `docker-compose.yml` | 部署配置不变，新增环境变量可选 |

---

## 11. 风险 & 回滚

### 风险

1. **旧客户端不兼容**：旧客户端收到新 LoginResponse 格式可能行为异常
   - 缓解：旧 token 直接失效，旧客户端会提示密码登录，不会数据损坏
2. **JWT_SECRET 泄露**：攻击者可伪造任意用户 token
   - 缓解：通过环境变量配置，不写入代码/日志
3. **时钟偏移**：JWT 过期依赖服务端时间
   - 缓解：单实例部署，无时钟同步问题

### 回滚方案

如需回滚到旧 token 系统：
1. 恢复旧版代码
2. 用户需重新密码登录（旧 token 已被清除）
3. `users.refresh_token` 列保留但不再使用（无害）

---

## 12. 后续优化（不在本批次范围）

- Token 版本号（`token_version`）：支持 token 族系追踪和旧 refresh token 检测
- Refresh token 轮换：每次 refresh 返回新 refresh token
- JWT 黑名单：支持主动撤销已签发的 access token
- RS256 签名：分布式部署时改用非对称加密
