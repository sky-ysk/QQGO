# v0.9: 历史消息查询 + 会话搜索 实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在现有历史消息翻页基础上，新增按时间范围查询历史消息和基于 FTS5 的全文搜索功能。

**Architecture:** 扩展 HistoryRequest 支持时间范围参数；创建 SQLite FTS5 虚拟表 + 触发器实现消息全文搜索；客户端新增 /history 和 /searchmsg 命令。

**Tech Stack:** Go, SQLite FTS5, GORM, gorilla/websocket

---

## 文件结构

| 文件 | 变更 | 说明 |
|------|------|------|
| `internal/model/message.go` | 修改 | 新增消息类型 315/316 + SearchRequest/SearchResponse/SearchResultItem，扩展 HistoryRequest |
| `internal/store/db.go` | 修改 | 新增 InitFTS 方法（FTS5 虚拟表 + 触发器 + 已有数据重建） |
| `internal/service/chat.go` | 修改 | 扩展 GetHistoryWithTarget 支持时间范围 + 新增 SearchMessages + getContextMessages |
| `internal/handler/ws.go` | 修改 | 扩展 handleHistory + dispatch 新增 MsgTypeSearchMessages case + 新增 handleSearchMessages |
| `cmd/client/main.go` | 修改 | 新增 /history 和 /searchmsg 命令 + displaySearchResults 函数 |
| `internal/service/chat_test.go` | 修改 | 新增历史查询时间范围测试 + FTS5 搜索测试 |
| `cmd/server/main.go` | 修改 | 启动时调用 InitFTS |

---

### Task 1: 数据模型扩展

**Files:**
- Modify: `internal/model/message.go:37-39` (消息类型)
- Modify: `internal/model/message.go:156-160` (HistoryRequest)
- Modify: `internal/model/message.go:269` (文件末尾新增结构体)

- [ ] **Step 1: 新增消息类型常量**

在 `MsgTypeGroupHistory = 314` 后新增：

```go
MsgTypeSearchMessages MessageType = 315
MsgTypeSearchResults  MessageType = 316
```

- [ ] **Step 2: 扩展 HistoryRequest 支持时间范围**

```go
type HistoryRequest struct {
	TargetQQ int64  `json:"target_qq"`
	Offset   int    `json:"offset"`
	Limit    int    `json:"limit"`
	FromTime string `json:"from_time,omitempty"`
	ToTime   string `json:"to_time,omitempty"`
}
```

- [ ] **Step 3: 新增搜索相关结构体（文件末尾）**

```go
type SearchRequest struct {
	Keyword  string `json:"keyword"`
	TargetQQ int64  `json:"target_qq,omitempty"`
	GroupID  string `json:"group_id,omitempty"`
	Limit    int    `json:"limit"`
}

type SearchResultItem struct {
	MessageID     int64            `json:"message_id"`
	FromQQ        int64            `json:"from_qq"`
	ToQQ          int64            `json:"to_qq"`
	GroupID       string           `json:"group_id"`
	Content       string           `json:"content"`
	CreatedAt     time.Time        `json:"created_at"`
	ContextBefore *HistoryMessage  `json:"context_before,omitempty"`
	ContextAfter  *HistoryMessage  `json:"context_after,omitempty"`
}

type SearchResponse struct {
	Keyword string             `json:"keyword"`
	Total   int                `json:"total"`
	Results []SearchResultItem `json:"results"`
}
```

- [ ] **Step 4: 编译验证**

Run: `go build ./...`
Expected: BUILD OK

- [ ] **Step 5: Commit**

```bash
git add internal/model/message.go
git commit -m "feat(v0.9): add search message types and extend HistoryRequest with time range"
```

---

### Task 2: FTS5 虚拟表初始化

**Files:**
- Modify: `internal/store/db.go` (新增 InitFTS 函数)
- Modify: `cmd/server/main.go:47-51` (调用 InitFTS)

- [ ] **Step 1: 在 `internal/store/db.go` 末尾新增 InitFTS 函数**

```go
func InitFTS(db *gorm.DB) error {
	// 创建 FTS5 虚拟表
	createFTS := `CREATE VIRTUAL TABLE IF NOT EXISTS messages_fts USING fts5(
		content,
		tokenize='unicode61'
	)`
	if err := db.Exec(createFTS).Error; err != nil {
		return fmt.Errorf("create fts table: %w", err)
	}

	// INSERT 触发器
	if err := db.Exec(`CREATE TRIGGER IF NOT EXISTS messages_fts_ai AFTER INSERT ON messages BEGIN
		INSERT INTO messages_fts(rowid, content) VALUES (new.id, new.content);
	END`).Error; err != nil {
		return fmt.Errorf("create insert trigger: %w", err)
	}

	// UPDATE 触发器
	if err := db.Exec(`CREATE TRIGGER IF NOT EXISTS messages_fts_au AFTER UPDATE OF content ON messages BEGIN
		UPDATE messages_fts SET content = new.content WHERE rowid = new.id;
	END`).Error; err != nil {
		return fmt.Errorf("create update trigger: %w", err)
	}

	// DELETE 触发器
	if err := db.Exec(`CREATE TRIGGER IF NOT EXISTS messages_fts_ad AFTER DELETE ON messages BEGIN
		DELETE FROM messages_fts WHERE rowid = old.id;
	END`).Error; err != nil {
		return fmt.Errorf("create delete trigger: %w", err)
	}

	// 重建已有数据索引（幂等：如果已有数据则重建，无数据则无影响）
	if err := db.Exec(`INSERT INTO messages_fts(messages_fts) VALUES('rebuild')`).Error; err != nil {
		return fmt.Errorf("rebuild fts index: %w", err)
	}

	return nil
}
```

需要在文件顶部 import 中确认 `"fmt"` 已存在（当前没有，需要添加）。

- [ ] **Step 2: 在 `cmd/server/main.go` 中 InitDB 后调用 InitFTS**

在 `store.InitDB` 之后、`service.NewChatService` 之前添加：

```go
if err := store.InitFTS(db); err != nil {
	log.Printf("[fts] init warning: %v (search may be unavailable)", err)
} else {
	log.Printf("[fts] full-text search initialized")
}
```

- [ ] **Step 3: 编译验证**

Run: `go build ./...`
Expected: BUILD OK

- [ ] **Step 4: Commit**

```bash
git add internal/store/db.go cmd/server/main.go
git commit -m "feat(v0.9): add FTS5 virtual table initialization with triggers"
```

---

### Task 3: 服务端 — 历史查询支持时间范围

**Files:**
- Modify: `internal/service/chat.go:502-523` (GetHistoryWithTarget 方法)
- Modify: `internal/handler/ws.go:805-857` (handleHistory 方法)

- [ ] **Step 1: 扩展 `GetHistoryWithTarget` 方法签名**

修改 `internal/service/chat.go:502`：

```go
func (s *ChatService) GetHistoryWithTarget(myQQ int64, targetQQ int64, offset int, limit int, fromTime string, toTime string) ([]*model.Message, bool, error) {
	var msgs []*model.Message
	query := s.db.Where(
		"((from_qq = ? AND to_qq = ?) OR (from_qq = ? AND to_qq = ?)) AND group_id = ''",
		myQQ, targetQQ, targetQQ, myQQ,
	).Where("msg_type IN ?", []int{1, 2, 3}).Where("is_recalled = ?", false)

	if fromTime != "" {
		query = query.Where("created_at >= ?", fromTime)
	}
	if toTime != "" {
		if len(toTime) == 10 {
			toTime = toTime + "T23:59:59"
		}
		query = query.Where("created_at <= ?", toTime)
	}

	var total int64
	query.Model(&model.Message{}).Count(&total)

	err := query.
		Order("id asc").
		Offset(offset).
		Limit(limit).
		Find(&msgs).Error
	if err != nil {
		return nil, false, err
	}

	hasMore := int64(offset+limit) < total
	return msgs, hasMore, nil
}
```

- [ ] **Step 2: 修改 `handleHistory` 传递时间参数**

修改 `internal/handler/ws.go:827` 的调用行：

```go
msgs, hasMore, err := h.svc.GetHistoryWithTarget(c.QQ, req.TargetQQ, req.Offset, req.Limit, req.FromTime, req.ToTime)
```

- [ ] **Step 3: 编译验证**

Run: `go build ./...`
Expected: BUILD OK

- [ ] **Step 4: 运行已有测试确保不破坏**

Run: `go test ./internal/service/... -run TestHistory -v`
Expected: 现有测试通过（如果有）或无匹配测试

- [ ] **Step 5: Commit**

```bash
git add internal/service/chat.go internal/handler/ws.go
git commit -m "feat(v0.9): support time range filter in GetHistoryWithTarget"
```

---

### Task 4: 服务端 — FTS5 全文搜索

**Files:**
- Modify: `internal/service/chat.go` (文件末尾新增方法)
- Modify: `internal/handler/ws.go` (dispatch switch + 新增 handleSearchMessages)

- [ ] **Step 1: 在 `internal/service/chat.go` 末尾新增 SearchMessages 方法**

```go
func (s *ChatService) SearchMessages(myQQ int64, keyword string, targetQQ int64, groupID string, limit int) (*model.SearchResponse, error) {
	if limit <= 0 {
		limit = 50
	}

	// 转义 FTS5 特殊字符
	escapedKeyword := escapeFTS5Keyword(keyword)

	var messages []*model.Message
	var err error

	if groupID != "" {
		// 群聊搜索
		err = s.db.Raw(`
			SELECT m.id, m.from_qq, m.to_qq, m.group_id, m.content, m.created_at
			FROM messages_fts f
			JOIN messages m ON f.rowid = m.id
			WHERE messages_fts MATCH ?
			  AND m.group_id = ?
			  AND m.msg_type IN (1, 2, 3)
			  AND m.is_recalled = 0
			ORDER BY rank
			LIMIT ?
		`, escapedKeyword, groupID, limit).Scan(&messages).Error
	} else if targetQQ != 0 {
		// 指定会话搜索
		err = s.db.Raw(`
			SELECT m.id, m.from_qq, m.to_qq, m.group_id, m.content, m.created_at
			FROM messages_fts f
			JOIN messages m ON f.rowid = m.id
			WHERE messages_fts MATCH ?
			  AND ((m.from_qq = ? AND m.to_qq = ?) OR (m.from_qq = ? AND m.to_qq = ?))
			  AND m.group_id = ''
			  AND m.msg_type IN (1, 2, 3)
			  AND m.is_recalled = 0
			ORDER BY rank
			LIMIT ?
		`, escapedKeyword, myQQ, targetQQ, targetQQ, myQQ, limit).Scan(&messages).Error
	} else {
		// 全局搜索（只搜索当前用户参与的消息）
		err = s.db.Raw(`
			SELECT m.id, m.from_qq, m.to_qq, m.group_id, m.content, m.created_at
			FROM messages_fts f
			JOIN messages m ON f.rowid = m.id
			WHERE messages_fts MATCH ?
			  AND (m.from_qq = ? OR m.to_qq = ?)
			  AND m.msg_type IN (1, 2, 3)
			  AND m.is_recalled = 0
			ORDER BY rank
			LIMIT ?
		`, escapedKeyword, myQQ, myQQ, limit).Scan(&messages).Error
	}

	if err != nil {
		return nil, err
	}

	results := make([]model.SearchResultItem, 0, len(messages))
	for _, m := range messages {
		before, after := s.getContextMessages(m.ID, m, myQQ)
		results = append(results, model.SearchResultItem{
			MessageID:     m.ID,
			FromQQ:        m.FromQQ,
			ToQQ:          m.ToQQ,
			GroupID:       m.GroupID,
			Content:       m.Content,
			CreatedAt:     m.CreatedAt,
			ContextBefore: before,
			ContextAfter:  after,
		})
	}

	return &model.SearchResponse{
		Keyword: keyword,
		Total:   len(results),
		Results: results,
	}, nil
}

func escapeFTS5Keyword(keyword string) string {
	// 将关键词用双引号包裹，避免 FTS5 特殊字符解析错误
	return `"` + strings.ReplaceAll(keyword, `"`, `""`) + `"`
}
```

需要在 `internal/service/chat.go` 顶部 import 中确认 `"strings"` 已存在。

- [ ] **Step 2: 新增 getContextMessages 方法**

在 SearchMessages 之后添加：

```go
func (s *ChatService) getContextMessages(messageID int64, msg *model.Message, myQQ int64) (*model.HistoryMessage, *model.HistoryMessage) {
	query := s.db.Table("messages").Where("msg_type IN ? AND is_recalled = ?", []int{1, 2, 3}, false)

	if msg.GroupID != "" {
		query = query.Where("group_id = ?", msg.GroupID)
	} else {
		query = query.Where(
			"((from_qq = ? AND to_qq = ?) OR (from_qq = ? AND to_qq = ?)) AND group_id = ''",
			msg.FromQQ, msg.ToQQ, msg.ToQQ, msg.FromQQ,
		)
	}

	var before model.HistoryMessage
	errBefore := query.Where("id < ?", messageID).Order("id DESC").Limit(1).First(&before).Error

	var after model.HistoryMessage
	errAfter := query.Where("id > ?", messageID).Order("id ASC").Limit(1).First(&after).Error

	var resultBefore, resultAfter *model.HistoryMessage
	if errBefore == nil {
		resultBefore = &before
	}
	if errAfter == nil {
		resultAfter = &after
	}

	return resultBefore, resultAfter
}
```

- [ ] **Step 3: 在 `internal/handler/ws.go` dispatch switch 中新增 case**

在 `case model.MsgTypeGroupHistory:` 之后（约第232行后）添加：

```go
case model.MsgTypeSearchMessages:
	h.handleSearchMessages(c, &msg)
```

- [ ] **Step 4: 新增 handleSearchMessages 方法**

在 `handleSessionList` 之后（约第937行后）添加：

```go
func (h *Hub) handleSearchMessages(c *ws.Conn, msg *model.Message) {
	if c.QQ == 0 {
		h.writeFriendError(c, "not logged in")
		return
	}

	var req model.SearchRequest
	if err := json.Unmarshal([]byte(msg.Content), &req); err != nil {
		h.writeFriendError(c, "invalid payload")
		return
	}

	if req.Keyword == "" {
		h.writeFriendError(c, "keyword is required")
		return
	}

	if len(req.Keyword) < 1 {
		h.writeFriendError(c, "keyword too short")
		return
	}

	resp, err := h.svc.SearchMessages(c.QQ, req.Keyword, req.TargetQQ, req.GroupID, req.Limit)
	if err != nil {
		log.Printf("[search] query error: %v", err)
		h.writeFriendError(c, "search failed")
		return
	}

	payload, _ := json.Marshal(resp)
	c.WriteJSON(&model.Message{
		MsgType: model.MsgTypeSearchResults,
		Content: string(payload),
	})
}
```

- [ ] **Step 5: 编译验证**

Run: `go build ./...`
Expected: BUILD OK

- [ ] **Step 6: Commit**

```bash
git add internal/service/chat.go internal/handler/ws.go
git commit -m "feat(v0.9): add FTS5 full-text search with context messages"
```

---

### Task 5: 客户端 — /history 命令（时间范围查询）

**Files:**
- Modify: `cmd/client/main.go` (命令处理 + requestHistory 函数 + displayHistory 函数)

- [ ] **Step 1: 修改 `requestHistory` 函数支持时间范围**

修改 `cmd/client/main.go:480`：

```go
func requestHistory(conn *websocket.Conn, targetQQ int64, offset int, fromTime string, toTime string) {
	payload, _ := json.Marshal(&model.HistoryRequest{
		TargetQQ: targetQQ,
		Offset:   offset,
		Limit:    30,
		FromTime: fromTime,
		ToTime:   toTime,
	})
	msg, _ := json.Marshal(&model.Message{
		MsgType: model.MsgTypeHistory,
		Content: string(payload),
	})
	conn.WriteMessage(websocket.TextMessage, msg)
}
```

- [ ] **Step 2: 新增全局变量存储当前历史查询的时间范围**

在 `cmd/client/main.go` 顶部全局变量区域（约第29-34行）添加：

```go
historyFromTime string
historyToTime   string
```

- [ ] **Step 3: 新增 /history 命令处理**

在命令 switch 中 `/sessions` case 之后添加：

```go
case "/history":
	if len(parts) < 2 {
		fmt.Println("[cmd] Usage: /history <qq> [--from YYYY-MM-DD] [--to YYYY-MM-DD]")
		return true
	}
	targetQQ, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		fmt.Println("[cmd] invalid QQ number")
		return true
	}
	var fromTime, toTime string
	for i := 2; i < len(parts); i++ {
		if parts[i] == "--from" && i+1 < len(parts) {
			fromTime = parts[i+1]
			i++
		} else if parts[i] == "--to" && i+1 < len(parts) {
			toTime = parts[i+1]
			i++
		}
	}
	historyTargetQQ = targetQQ
	historyOffset = 0
	historyFromTime = fromTime
	historyToTime = toTime
	historyGroupID = ""
	requestHistory(conn, targetQQ, 0, fromTime, toTime)
```

- [ ] **Step 4: 修改 /prev 和 /next 命令，传递时间范围**

修改 `/prev` case（约第158行）：

```go
case "/prev":
	if historyGroupID != "" {
		historyOffset += 30
		requestGroupHistory(conn, historyGroupID, historyOffset)
	} else if historyTargetQQ != 0 {
		historyOffset += 30
		requestHistory(conn, historyTargetQQ, historyOffset, historyFromTime, historyToTime)
	} else {
		fmt.Println("[cmd] no chat history context, use /to <qq_number> or /togroup <group_id> first")
	}
```

修改 `/next` case（约第169行）：

```go
case "/next":
	if historyGroupID != "" {
		if historyOffset >= 30 {
			historyOffset -= 30
		} else {
			historyOffset = 0
		}
		requestGroupHistory(conn, historyGroupID, historyOffset)
	} else if historyTargetQQ != 0 {
		if historyOffset >= 30 {
			historyOffset -= 30
		} else {
			historyOffset = 0
		}
		requestHistory(conn, historyTargetQQ, historyOffset, historyFromTime, historyToTime)
	} else {
		fmt.Println("[cmd] no chat history context, use /to <qq_number> or /togroup <group_id> first")
	}
```

- [ ] **Step 5: 修改 /to 命令中自动拉取历史的调用**

找到 `requestHistory(conn, resp.QQNumber, 0)` 的调用（约在 handleCheckUser 响应处理处），改为：

```go
requestHistory(conn, resp.QQNumber, 0, "", "")
```

- [ ] **Step 6: 修改 displayHistory 显示时间范围**

修改 `displayHistory` 函数（约第1239行）：

```go
func displayHistory(resp model.HistoryResponse) {
	if len(resp.Messages) == 0 && resp.Offset == 0 {
		return
	}

	title := fmt.Sprintf("\n───── History with %s (QQ:%d)", resp.Nickname, resp.TargetQQ)
	if historyFromTime != "" || historyToTime != "" {
		title += fmt.Sprintf(" [%s ~ %s]", func() string {
			f := historyFromTime
			if f == "" {
				f = "..."
			}
			t := historyToTime
			if t == "" {
				t = "..."
			}
			return f + " ~ " + t
		}())
	}
	fmt.Println(title + " ─────")
	for _, m := range resp.Messages {
		timeStr := m.CreatedAt.Format("15:04:05")
		if m.FromQQ == myQQNumber {
			fmt.Printf("  [我]    %s  %s\n", timeStr, m.Content)
		} else {
			fmt.Printf("  [%s] %s  %s\n", resp.Nickname, timeStr, m.Content)
		}
	}
	if resp.HasMore {
		fmt.Println("  ... (use /prev for older messages)")
	}
	if resp.Offset > 0 {
		fmt.Println("  (use /next for newer messages)")
	}
	fmt.Println("─────────────────────────────────────")
}
```

- [ ] **Step 7: 编译验证**

Run: `go build ./...`
Expected: BUILD OK

- [ ] **Step 8: Commit**

```bash
git add cmd/client/main.go
git commit -m "feat(v0.9): add /history command with --from/--to time range filter"
```

---

### Task 6: 客户端 — /searchmsg 命令 + 搜索结果展示

**Files:**
- Modify: `cmd/client/main.go` (命令处理 + 请求函数 + 显示函数 + 消息接收处理)

- [ ] **Step 1: 新增 searchMessages 请求函数**

在 `requestGroupHistory` 之后添加：

```go
func searchMessages(conn *websocket.Conn, keyword string, targetQQ int64) {
	payload, _ := json.Marshal(&model.SearchRequest{
		Keyword:  keyword,
		TargetQQ: targetQQ,
		Limit:    50,
	})
	msg, _ := json.Marshal(&model.Message{
		MsgType: model.MsgTypeSearchMessages,
		Content: string(payload),
	})
	conn.WriteMessage(websocket.TextMessage, msg)
}
```

- [ ] **Step 2: 新增 /searchmsg 命令处理**

在 `/history` case 之后添加：

```go
case "/searchmsg":
	if len(parts) < 2 {
		fmt.Println("[cmd] Usage: /searchmsg <keyword> [qq]")
		return true
	}
	keyword := parts[1]
	var targetQQ int64
	if len(parts) >= 3 {
		targetQQ, _ = strconv.ParseInt(parts[2], 10, 64)
	}
	searchMessages(conn, keyword, targetQQ)
```

- [ ] **Step 3: 在消息接收 switch 中新增 MsgTypeSearchResults 处理**

在 `case model.MsgTypeGroupHistory:` 之后添加：

```go
case model.MsgTypeSearchResults:
	var resp model.SearchResponse
	json.Unmarshal([]byte(msg.Content), &resp)
	displaySearchResults(resp)
```

- [ ] **Step 4: 新增 displaySearchResults 函数**

在 `displayGroupHistory` 之后添加：

```go
func displaySearchResults(resp model.SearchResponse) {
	fmt.Printf("\n───── Search Results: \"%s\" (%d found) ─────\n", resp.Keyword, resp.Total)
	if len(resp.Results) == 0 {
		fmt.Println("  (no results)")
		fmt.Println("─────────────────────────────────────────────")
		return
	}

	for i, item := range resp.Results {
		if i > 0 {
			fmt.Println()
		}

		// 上下文：前一条
		if item.ContextBefore != nil {
			timeStr := item.ContextBefore.CreatedAt.Format("01-02 15:04")
			sender := fmt.Sprintf("%d", item.ContextBefore.FromQQ)
			if item.ContextBefore.FromQQ == myQQNumber {
				sender = "我"
			}
			fmt.Printf("    [%s] %s  %s\n", sender, timeStr, item.ContextBefore.Content)
		}

		// 匹配消息
		timeStr := item.CreatedAt.Format("01-02 15:04")
		sender := fmt.Sprintf("%d", item.FromQQ)
		if item.FromQQ == myQQNumber {
			sender = "我"
		}
		fmt.Printf("  > [%s] %s  %s\n", sender, timeStr, item.Content)

		// 上下文：后一条
		if item.ContextAfter != nil {
			timeStr := item.ContextAfter.CreatedAt.Format("01-02 15:04")
			sender := fmt.Sprintf("%d", item.ContextAfter.FromQQ)
			if item.ContextAfter.FromQQ == myQQNumber {
				sender = "我"
			}
			fmt.Printf("    [%s] %s  %s\n", sender, timeStr, item.ContextAfter.Content)
		}
	}
	fmt.Println("─────────────────────────────────────────────")
}
```

- [ ] **Step 5: 编译验证**

Run: `go build ./...`
Expected: BUILD OK

- [ ] **Step 6: Commit**

```bash
git add cmd/client/main.go
git commit -m "feat(v0.9): add /searchmsg command with context display"
```

---

### Task 7: 测试 — 历史查询时间范围

**Files:**
- Modify: `internal/service/chat_test.go` (文件末尾新增测试)

- [ ] **Step 1: 新增 TestHistoryTimeRange 测试**

```go
func TestHistoryTimeRange(t *testing.T) {
	db := setupTestDB(t)
	svc := NewChatService(db)

	qq1, _ := svc.Register("alice", "password123")
	qq2, _ := svc.Register("bob", "password456")

	svc.AcceptFriend(qq2, qq1)

	// 插入不同时间的消息
	now := time.Now()
	msgs := []model.Message{
		{MsgType: 1, FromQQ: qq1, ToQQ: qq2, Content: "msg1", CreatedAt: now.Add(-48 * time.Hour)},
		{MsgType: 1, FromQQ: qq2, ToQQ: qq1, Content: "msg2", CreatedAt: now.Add(-24 * time.Hour)},
		{MsgType: 1, FromQQ: qq1, ToQQ: qq2, Content: "msg3", CreatedAt: now.Add(-1 * time.Hour)},
	}
	for _, m := range msgs {
		svc.db.Create(&m)
	}

	// 测试：无时间范围，返回全部
	result, hasMore, err := svc.GetHistoryWithTarget(qq1, qq2, 0, 10, "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(result))
	}
	if hasMore {
		t.Fatal("should not have more")
	}

	// 测试：仅 --from，返回最近24小时
	fromTime := now.Add(-30 * time.Hour).Format("2006-01-02T15:04:05")
	result, _, err = svc.GetHistoryWithTarget(qq1, qq2, 0, 10, fromTime, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 messages with --from, got %d", len(result))
	}

	// 测试：--from + --to，返回指定范围
	toTime := now.Add(-12 * time.Hour).Format("2006-01-02")
	result, _, err = svc.GetHistoryWithTarget(qq1, qq2, 0, 10, fromTime, toTime)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 message with --from/--to, got %d", len(result))
	}
	if result[0].Content != "msg2" {
		t.Fatalf("expected msg2, got %s", result[0].Content)
	}

	// 测试：日期格式 --to 包含当天全天
	toTimeDate := now.Format("2006-01-02")
	result, _, err = svc.GetHistoryWithTarget(qq1, qq2, 0, 10, fromTime, toTimeDate)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 messages with date --to, got %d", len(result))
	}

	// 测试：无匹配结果
	futureFrom := now.Add(24 * time.Hour).Format("2006-01-02")
	result, hasMore, err = svc.GetHistoryWithTarget(qq1, qq2, 0, 10, futureFrom, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Fatalf("expected 0 messages, got %d", len(result))
	}
	if hasMore {
		t.Fatal("should not have more when no results")
	}
}
```

- [ ] **Step 2: 运行测试**

Run: `go test ./internal/service/... -run TestHistoryTimeRange -v`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add internal/service/chat_test.go
git commit -m "test(v0.9): add time range filter tests for GetHistoryWithTarget"
```

---

### Task 8: 测试 — FTS5 全文搜索

**Files:**
- Modify: `internal/service/chat_test.go` (文件末尾新增测试)
- Modify: `internal/service/chat_test.go:15-38` (setupTestDB 需要初始化 FTS)

- [ ] **Step 1: 修改 setupTestDB 初始化 FTS**

在 `setupTestDB` 函数末尾（`InitJWT` 之后）添加：

```go
if err := store.InitFTS(db); err != nil {
	t.Logf("FTS init warning: %v (search tests may fail)", err)
}
```

需要在 import 中添加 `"github.com/qqgo/server/internal/store"`。

- [ ] **Step 2: 新增 TestSearchMessages 测试**

```go
func TestSearchMessages(t *testing.T) {
	db := setupTestDB(t)
	svc := NewChatService(db)

	qq1, _ := svc.Register("alice", "password123")
	qq2, _ := svc.Register("bob", "password456")
	qq3, _ := svc.Register("charlie", "password789")

	svc.AcceptFriend(qq2, qq1)

	// 插入消息
	now := time.Now()
	msgs := []model.Message{
		{MsgType: 1, FromQQ: qq1, ToQQ: qq2, Content: "你好，项目进度怎么样了？", CreatedAt: now.Add(-2 * time.Hour)},
		{MsgType: 1, FromQQ: qq2, ToQQ: qq1, Content: "项目已经完成80%了", CreatedAt: now.Add(-1 * time.Hour)},
		{MsgType: 1, FromQQ: qq1, ToQQ: qq2, Content: "太好了，下周上线", CreatedAt: now.Add(-30 * time.Minute)},
		{MsgType: 1, FromQQ: qq1, ToQQ: qq3, Content: "新项目什么时候开始", CreatedAt: now.Add(-10 * time.Minute)},
	}
	for _, m := range msgs {
		svc.db.Create(&m)
	}

	// 等待触发器同步
	time.Sleep(100 * time.Millisecond)

	// 测试：全局搜索"项目"
	resp, err := svc.SearchMessages(qq1, "项目", 0, "", 50)
	if err != nil {
		t.Fatalf("search error: %v", err)
	}
	if resp.Total < 3 {
		t.Fatalf("expected at least 3 results for '项目', got %d", resp.Total)
	}

	// 测试：指定会话搜索"项目"（只搜索 alice-bob 会话）
	resp, err = svc.SearchMessages(qq1, "项目", qq2, "", 50)
	if err != nil {
		t.Fatalf("search error: %v", err)
	}
	if resp.Total < 2 {
		t.Fatalf("expected at least 2 results in private chat, got %d", resp.Total)
	}
	// 验证不包含与 charlie 的消息
	for _, r := range resp.Results {
		if r.ToQQ == qq3 || r.FromQQ == qq3 {
			t.Fatal("should not include messages with charlie in private search")
		}
	}

	// 测试：搜索无结果
	resp, err = svc.SearchMessages(qq1, "不存在的关键词", 0, "", 50)
	if err != nil {
		t.Fatalf("search error: %v", err)
	}
	if resp.Total != 0 {
		t.Fatalf("expected 0 results, got %d", resp.Total)
	}

	// 测试：上下文正确性
	resp, err = svc.SearchMessages(qq1, "完成", qq2, "", 50)
	if err != nil {
		t.Fatalf("search error: %v", err)
	}
	if resp.Total < 1 {
		t.Fatal("expected at least 1 result for '完成'")
	}
	firstResult := resp.Results[0]
	if firstResult.ContextBefore == nil {
		t.Fatal("expected context before")
	}
	if firstResult.ContextBefore.Content != "你好，项目进度怎么样了？" {
		t.Fatalf("unexpected context before: %s", firstResult.ContextBefore.Content)
	}
	if firstResult.ContextAfter == nil {
		t.Fatal("expected context after")
	}
	if firstResult.ContextAfter.Content != "太好了，下周上线" {
		t.Fatalf("unexpected context after: %s", firstResult.ContextAfter.Content)
	}
}
```

- [ ] **Step 3: 新增 TestSearchRecalledMessages 测试**

```go
func TestSearchRecalledMessages(t *testing.T) {
	db := setupTestDB(t)
	svc := NewChatService(db)

	qq1, _ := svc.Register("alice", "password123")
	qq2, _ := svc.Register("bob", "password456")
	svc.AcceptFriend(qq2, qq1)

	now := time.Now()
	msgs := []model.Message{
		{MsgType: 1, FromQQ: qq1, ToQQ: qq2, Content: "这条消息会被撤回", CreatedAt: now.Add(-1 * time.Hour)},
		{MsgType: 1, FromQQ: qq1, ToQQ: qq2, Content: "这条消息正常", CreatedAt: now},
	}
	for _, m := range msgs {
		svc.db.Create(&m)
	}

	// 撤回第一条消息
	svc.db.Model(&model.Message{}).Where("content = ?", "这条消息会被撤回").Update("is_recalled", true)

	time.Sleep(100 * time.Millisecond)

	resp, err := svc.SearchMessages(qq1, "撤回", 0, "", 50)
	if err != nil {
		t.Fatalf("search error: %v", err)
	}
	if resp.Total != 0 {
		t.Fatalf("recalled messages should not appear in search, got %d results", resp.Total)
	}
}
```

- [ ] **Step 4: 运行测试**

Run: `go test ./internal/service/... -run TestSearch -v`
Expected: PASS (all search tests)

- [ ] **Step 5: 运行全部测试**

Run: `go test ./internal/... -count=1`
Expected: ALL PASS

- [ ] **Step 6: Commit**

```bash
git add internal/service/chat_test.go
git commit -m "test(v0.9): add FTS5 search tests with context and recalled message filtering"
```

---

### Task 9: 最终验证 + 文档更新

- [ ] **Step 1: 运行全部测试**

Run: `go test ./internal/... -count=1 -v`
Expected: ALL PASS

- [ ] **Step 2: go vet**

Run: `go vet ./...`
Expected: 无输出（无问题）

- [ ] **Step 3: 更新 CHANGELOG.md**

在文件顶部（`# CHANGELOG — QQGO 版本变更记录` 之后）添加：

```markdown
## [v0.9] — 2026-05-29 / branch: `feature/v0.9-history-search`

### Added
- **历史消息时间范围查询：** `/history <qq> --from YYYY-MM-DD --to YYYY-MM-DD` 命令，支持按时间范围筛选历史消息；`/prev` `/next` 翻页保持时间范围
- **全文搜索：** `/searchmsg <关键词> [qq]` 命令，支持全局搜索和指定会话搜索；SQLite FTS5 虚拟表 + 触发器自动同步
- **搜索上下文：** 搜索结果每条匹配消息显示前后各 1 条上下文消息
- **新消息类型：** `MsgTypeSearchMessages(315)`, `MsgTypeSearchResults(316)`
- **新数据模型：** `SearchRequest`, `SearchResponse`, `SearchResultItem`；`HistoryRequest` 扩展 `FromTime`/`ToTime` 字段
- **FTS5 基础设施：** `messages_fts` 虚拟表 + INSERT/UPDATE/DELETE 触发器 + 已有数据自动重建索引

### Changed
- **GetHistoryWithTarget：** 新增 `fromTime`/`toTime` 参数，支持时间范围过滤
- **handleHistory：** 传递时间范围参数到 service 层
- **displayHistory：** 标题显示时间范围（如有）
- **requestHistory：** 支持 fromTime/toTime 参数
- **setupTestDB：** 初始化 FTS5 虚拟表

### New Files
- 无（均为修改已有文件）
```

- [ ] **Step 4: 更新 REQUIREMENTS.md**

将"历史消息查询"和"会话搜索"的状态从 `🔲 pending` 改为 `✅ done`，完成版本填 `v0.9`。

- [ ] **Step 5: 更新 Project.md**

在版本列表中新增 v0.9 条目。

- [ ] **Step 6: Commit**

```bash
git add CHANGELOG.md REQUIREMENTS.md Project.md
git commit -m "docs(v0.9): update changelog and requirements"
```

---

## 自审检查

**Spec 覆盖：**
- ✅ 历史消息查询（时间范围 + 会话）→ Task 3, Task 5
- ✅ 会话搜索（全局 + 指定会话）→ Task 4, Task 6
- ✅ FTS5 虚拟表 + 触发器 → Task 2
- ✅ 上下文消息 → Task 4 (getContextMessages), Task 6 (displaySearchResults)
- ✅ 数据模型 → Task 1
- ✅ 错误处理（关键词为空/过短）→ Task 4 (handleSearchMessages)
- ✅ 测试 → Task 7, Task 8

**Placeholder 扫描：** 无 TBD/TODO，所有代码块包含完整实现。

**类型一致性：**
- `HistoryRequest.FromTime`/`ToTime` 在 model → handler → service → client 中一致使用 string 类型
- `SearchRequest.Keyword`/`TargetQQ`/`GroupID`/`Limit` 在各层一致
- `SearchResponse.Keyword`/`Total`/`Results` 在各层一致
- `getContextMessages` 返回 `*HistoryMessage` 与 `SearchResultItem.ContextBefore/After` 类型匹配

**注意事项：**
- spec 第5节（数据库迁移）中的 `content='messages'` 外部内容表设计已被修正为普通 FTS 表 + 触发器方案（见 brainstorming 阶段的修正）
- `escapeFTS5Keyword` 用双引号包裹关键词避免 FTS5 特殊字符解析错误
- 全局搜索限定为当前用户参与的消息（`from_qq = myQQ OR to_qq = myQQ`），避免搜索到其他用户的消息
