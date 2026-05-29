# v0.9: 历史消息查询 + 会话搜索

**日期：** 2026-05-29
**状态：** 设计完成，待实现
**版本：** v0.9

---

## 1. 概述

在现有历史消息翻页（`/prev` `/next`）基础上，新增两个 P1 功能：

1. **历史消息查询**：按时间范围 + 指定会话查询历史消息
2. **会话搜索**：在历史消息中按关键词搜索内容，支持全局搜索和指定会话搜索

搜索功能使用 SQLite FTS5 全文搜索引擎，提供高性能的文本搜索能力。

---

## 2. 架构概览

```
客户端命令                        服务端处理                          数据层
─────────                        ──────────                          ─────
/history <qq> --from --to    →  handleHistoryWithRange      →  messages 表 (created_at 索引)
/searchmsg <关键词> [qq]     →  handleSearchMessages        →  messages_fts (FTS5 虚拟表)
```

### 核心变更

| 组件 | 变更内容 |
|------|----------|
| 服务端 handler | 扩展 `handleHistory` 支持时间范围 + 新增 `handleSearchMessages` |
| 服务端 service | `GetHistoryWithTarget` 增加时间过滤 + 新增 `SearchMessages` 方法 |
| 服务端 store | 创建 FTS5 虚拟表 + 消息入库时同步到 FTS 表 |
| 客户端 | 新增 `/history` 和 `/searchmsg` 命令 + 搜索结果展示 |
| 数据模型 | 新增 `SearchRequest`/`SearchResponse` + 扩展 `HistoryRequest` |
| 消息类型 | 新增 `MsgTypeSearchMessages(315)` / `MsgTypeSearchResults(316)` |

---

## 3. 历史消息查询（按时间范围 + 会话）

### 3.1 命令格式

```
/history <qq>                                          # 查看与指定QQ的所有历史
/history <qq> --from 2026-05-01                        # 从指定日期开始
/history <qq> --to 2026-05-28                          # 到指定日期为止
/history <qq> --from 2026-05-01 --to 2026-05-28        # 时间范围
```

### 3.2 服务端变更

#### HistoryRequest 扩展

```go
type HistoryRequest struct {
    TargetQQ int64  `json:"target_qq"`
    Offset   int    `json:"offset"`
    Limit    int    `json:"limit"`
    FromTime string `json:"from_time,omitempty"`  // "2006-01-02" 或 "2006-01-02T15:04:05"
    ToTime   string `json:"to_time,omitempty"`
}
```

#### GetHistoryWithTarget 变更

在 `internal/service/chat.go` 中扩展查询：

```go
func (s *ChatService) GetHistoryWithTarget(myQQ, targetQQ int64, offset, limit int, fromTime, toTime string) ([]*model.Message, bool, error) {
    query := s.db.Where(
        "((from_qq = ? AND to_qq = ?) OR (from_qq = ? AND to_qq = ?)) AND group_id = ''",
        myQQ, targetQQ, targetQQ, myQQ,
    ).Where("msg_type IN ? AND is_recalled = ?", []int32{1, 2, 3}, false)

    // 时间范围过滤
    if fromTime != "" {
        query = query.Where("created_at >= ?", fromTime)
    }
    if toTime != "" {
        // 如果只指定了日期，加 23:59:59
        if len(toTime) == 10 {
            toTime = toTime + "T23:59:59"
        }
        query = query.Where("created_at <= ?", toTime)
    }

    var messages []*model.Message
    err := query.Order("id ASC").Offset(offset).Limit(limit + 1).Find(&messages).Error
    // ... hasMore 逻辑不变
}
```

### 3.3 客户端变更

- 新增 `/history` 命令解析（支持 `--from` / `--to` 参数）
- 发送 `MsgTypeHistory` 请求，携带时间范围
- 显示格式复用现有 `displayHistory`，标题增加时间范围显示

### 3.4 交互示例

```
> /history 10002 --from 2026-05-01 --to 2026-05-15

───── History with Bob (QQ:10002) [2026-05-01 ~ 2026-05-15] ─────
[我]    05-01 10:30  你好！
[Bob]   05-01 10:31  嗨，最近怎么样？
[我]    05-01 10:32  还不错
...
──────────────────────────────────────────────────────────────────
(use /prev for older messages, /next for newer messages)
```

---

## 4. 会话搜索（FTS5 全文搜索）

### 4.1 命令格式

```
/searchmsg <关键词>              # 全局搜索所有消息
/searchmsg <关键词> <qq>         # 在指定会话中搜索
```

### 4.2 FTS5 虚拟表设计

#### 创建虚拟表

```sql
CREATE VIRTUAL TABLE IF NOT EXISTS messages_fts USING fts5(
    content,
    tokenize='unicode61'
);
```

| 配置项 | 说明 |
|--------|------|
| `content` | 消息文本内容副本（图片/文件消息存储文件名） |
| `tokenize='unicode61'` | Unicode 分词器，支持中英文（中文按字符分词） |

FTS 表存储消息内容副本，通过触发器与 messages 表保持同步。rowid 与 messages.id 对应。

#### 数据同步

FTS 表存储消息内容副本，通过 SQLite 触发器与 messages 表保持同步：

```sql
-- INSERT 触发器
CREATE TRIGGER IF NOT EXISTS messages_fts_ai AFTER INSERT ON messages BEGIN
    INSERT INTO messages_fts(rowid, content) VALUES (new.id, new.content);
END;

-- UPDATE 触发器
CREATE TRIGGER IF NOT EXISTS messages_fts_au AFTER UPDATE OF content ON messages BEGIN
    UPDATE messages_fts SET content = new.content WHERE rowid = new.id;
END;

-- DELETE 触发器
CREATE TRIGGER IF NOT EXISTS messages_fts_ad AFTER DELETE ON messages BEGIN
    DELETE FROM messages_fts WHERE rowid = old.id;
END;
```

触发器在 `InitFTS` 中一并创建。写入消息时无需额外代码，触发器自动维护 FTS 索引。

#### 搜索查询

**全局搜索：**
```sql
SELECT m.id, m.from_qq, m.to_qq, m.group_id, m.content, m.created_at
FROM messages_fts f
JOIN messages m ON f.rowid = m.id
WHERE messages_fts MATCH '关键词'
  AND m.msg_type IN (1, 2, 3)
  AND m.is_recalled = 0
ORDER BY rank
LIMIT 50
```

**指定会话搜索：**
```sql
SELECT m.id, m.from_qq, m.to_qq, m.group_id, m.content, m.created_at
FROM messages_fts f
JOIN messages m ON f.rowid = m.id
WHERE messages_fts MATCH '关键词'
  AND (
    (m.from_qq = ? AND m.to_qq = ?) OR 
    (m.from_qq = ? AND m.to_qq = ?)
  )
  AND m.group_id = ''
  AND m.msg_type IN (1, 2, 3)
  AND m.is_recalled = 0
ORDER BY rank
LIMIT 50
```

**群聊搜索：**
```sql
SELECT m.id, m.from_qq, m.to_qq, m.group_id, m.content, m.created_at
FROM messages_fts f
JOIN messages m ON f.rowid = m.id
WHERE messages_fts MATCH '关键词'
  AND m.group_id = ?
  AND m.msg_type IN (1, 2, 3)
  AND m.is_recalled = 0
ORDER BY rank
LIMIT 50
```

### 4.3 上下文获取

对每个匹配的消息，获取前后各 1 条消息作为上下文：

```go
func (s *ChatService) getContextMessages(messageID int64, msg *model.Message, myQQ int64) (*model.HistoryMessage, *model.HistoryMessage, error) {
    // 根据消息类型确定会话范围
    query := s.db.Table("messages").Where("msg_type IN ? AND is_recalled = ?", []int32{1,2,3}, false)
    
    if msg.GroupID != "" {
        // 群聊：同一群内
        query = query.Where("group_id = ?", msg.GroupID)
    } else {
        // 私聊：同一会话（双向）
        query = query.Where(
            "((from_qq = ? AND to_qq = ?) OR (from_qq = ? AND to_qq = ?)) AND group_id = ''",
            msg.FromQQ, msg.ToQQ, msg.ToQQ, msg.FromQQ,
        )
    }

    // 获取前一条
    var before *model.HistoryMessage
    query.Where("id < ?", messageID).Order("id DESC").Limit(1).First(&before)

    // 获取后一条
    var after *model.HistoryMessage
    query.Where("id > ?", messageID).Order("id ASC").Limit(1).First(&after)

    return before, after, nil
}
```

### 4.4 数据模型

```go
type SearchRequest struct {
    Keyword  string `json:"keyword"`
    TargetQQ int64  `json:"target_qq,omitempty"`  // 0 = 全局搜索
    GroupID  string `json:"group_id,omitempty"`   // 群ID，非空则搜索群聊
    Limit    int    `json:"limit"`                // 默认 50
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

### 4.5 新增消息类型

```go
MsgTypeSearchMessages = 315  // 搜索消息请求
MsgTypeSearchResults  = 316  // 搜索结果响应
```

### 4.6 服务端处理流程

```
handleSearchMessages:
1. 解析 SearchRequest
2. 如果 TargetQQ != 0 → 指定会话搜索
3. 如果 GroupID != "" → 群聊搜索
4. 否则 → 全局搜索
5. 调用 svc.SearchMessages() 获取匹配消息 ID 列表
6. 对每个 ID，查询完整消息 + 上下文
7. 返回 SearchResponse
```

### 4.7 客户端显示

```
> /searchmsg 项目

───── Search Results: "项目" (3 found) ─────

[Bob]   05-10 14:30  那个项目进度怎么样了？
[我]    05-10 14:31  项目已经完成80%了
[Bob]   05-10 14:32  太好了

[Alice] 05-12 09:15  新项目什么时候开始？
[我]    05-12 09:16  下周开始新项目
[Alice] 05-12 09:17  OK

─────────────────────────────────────────────
```

每条匹配消息显示前后各 1 条上下文，上下文消息用缩进或浅色区分。

---

## 5. 数据库迁移

### 5.1 FTS5 虚拟表创建

在 `internal/store/db.go` 的初始化逻辑中添加：

```go
func InitFTS(db *gorm.DB) error {
    // 创建 FTS5 虚拟表
    sql := `CREATE VIRTUAL TABLE IF NOT EXISTS messages_fts USING fts5(
        content,
        content='messages',
        content_rowid='id',
        tokenize='unicode61'
    )`
    return db.Exec(sql).Error
}
```

### 5.2 已有数据重建索引

对于已有消息数据，首次启动时需要重建 FTS 索引：

```sql
INSERT INTO messages_fts(messages_fts) VALUES('rebuild');
```

### 5.3 SQLite FTS5 支持检查

`modernc.org/sqlite`（纯 Go SQLite 驱动）默认编译包含 FTS5 扩展，无需额外配置。

---

## 6. 错误处理

| 场景 | 行为 |
|------|------|
| 关键词为空 | 返回错误："keyword is required" |
| 关键词过短（< 1字符） | 返回错误："keyword too short" |
| 时间格式错误 | 返回错误："invalid time format, use YYYY-MM-DD" |
| FTS5 不可用 | 降级为 LIKE 查询，日志警告 |
| 搜索结果过多 | 截断到 Limit 条，返回 Total 实际匹配数 |

---

## 7. 测试计划

### 7.1 历史消息查询测试

| 测试用例 | 验证点 |
|----------|--------|
| 无时间范围查询 | 行为与现有逻辑一致 |
| 仅 --from | 返回 from 之后的消息 |
| 仅 --to | 返回 to 之前的消息 |
| --from + --to | 返回时间范围内的消息 |
| 日期格式 vs 时间格式 | "2006-01-02" 和 "2006-01-02T15:04:05" 均正确 |
| --to 日期包含当天全天 | to="2026-05-28" 等效于 to="2026-05-28T23:59:59" |
| 时间范围内无消息 | 返回空列表，hasMore=false |
| 翻页 + 时间范围 | offset 在时间范围过滤后生效 |

### 7.2 会话搜索测试

| 测试用例 | 验证点 |
|----------|--------|
| 全局搜索有结果 | 返回匹配消息 + 上下文 |
| 全局搜索无结果 | 返回空列表 |
| 指定会话搜索 | 只返回指定会话的匹配消息 |
| 群聊搜索 | 只返回指定群的匹配消息 |
| 中文搜索 | unicode61 分词器正确处理中文 |
| 关键词过短 | 返回错误 |
| 关键词为空 | 返回错误 |
| 上下文正确性 | before 是 id-1 方向的消息，after 是 id+1 方向 |
| 上下文边界 | 第一条/最后一条消息的上下文为 nil |
| 撤回消息不出现在结果 | is_recalled=true 的消息不被搜索到 |
| FTS5 索引重建 | 已有数据能正确重建索引 |

---

## 8. 涉及文件

| 文件 | 变更类型 | 说明 |
|------|----------|------|
| `internal/model/message.go` | 修改 | 新增 SearchRequest/SearchResponse/SearchResultItem，扩展 HistoryRequest |
| `internal/service/chat.go` | 修改 | 扩展 GetHistoryWithTarget + 新增 SearchMessages + getContextMessages |
| `internal/store/db.go` | 修改 | 新增 InitFTS 方法 |
| `internal/handler/ws.go` | 修改 | 扩展 handleHistory + 新增 handleSearchMessages |
| `cmd/client/main.go` | 修改 | 新增 /history 和 /searchmsg 命令 + displaySearchResults |
| `internal/service/chat_test.go` | 新增 | 历史查询 + 搜索测试 |

---

## 9. 后续可扩展方向（不在本次范围）

- 搜索结果高亮（标记关键词）
- 搜索历史（保存最近搜索关键词）
- 多关键词搜索（AND/OR 逻辑）
- 搜索结果按会话分组展示
- 搜索消息类型过滤（仅文本/仅图片等）
