项目目录为 /Users/yangshikang.6/Desktop/Code/Go/QQGO
请先阅读以下文档了解项目当前状态：
- Project.md（架构文档 + 版本历史）
- REQUIREMENTS.md（需求 backlog + 各版本计划详情）
- BUGS.md（缺陷追踪）
- CHANGELOG.md（版本发布记录）
- docs/superpowers/tests/2026-05-26-v0.7-test-report.md（v0.7 测试报告）
当前版本：v0.7（已完成，main 分支）
下一版本：v0.8
v0.5 已规划的需求（来自 REQUIREMENTS.md）：
1. 非好友消息限制 — 好友自由发消息，非好友只能发 1 条
2. 会话历史记录 — /to 后显示最近 30 条，支持 /prev /next 翻页
3. 群组聊天 — 创建群、加群、群消息广播
4. 会话列表 — /sessions 列出所有对话窗口
我想从 [具体需求名称] 开始设计。请先分析现有代码架构，然后给出设计方案。
关键点：
- 
明确列出需要读的文档路径
- 
说明当前版本和下一版本
- 
列出已规划的需求清单
- 
指定从哪个需求开始
- 
要求先分析代码再设计

# 2026-05-26 v0.7 开发完成记录
- v0.7 全部 9 项需求已完成（2026-05-26），20 个测试全部通过。详见 docs/superpowers/tests/2026-05-26-v0.7-test-report.md

v0.7 已完成项：
- ✅ Server 优雅退出（srv.Shutdown + Hub.Shutdown + DB.Close）
- ✅ SQLite GREATEST 兼容（GREATEST → MAX）
- ✅ BUG-005 好友上限 500 测试（TestFriendLimit500）
- ✅ BUG-006 离线好友请求测试（TestOfflineFriendRequest）
- ✅ 修改密码（/changepw + Token 刷新）
- ✅ 黑名单（blacklists 表 + /block /unblock /blacklist）
- ✅ 图片/文件消息（/sendimg /sendfile + base64 ≤5MB）
- ✅ 消息已读/未读（read_at 字段 + 自动回执）
- ✅ 消息撤回（is_recalled + 2 分钟限制 + 历史过滤）

v0.8 计划：架构升级 & 生产就绪
聚焦性能、安全、部署，为多实例和真实环境做准备。
需求	说明
Protobuf 协议	替换 JSON，定义 .proto，客户端/服务端双端适配
在线状态服务	Redis 缓存在线状态，支持 GET/SET，掉线自动过期
分布式消息路由	NATS 或 Redis PubSub 实现跨实例消息转发
TLS/SSL	WebSocket wss://，Let's Encrypt 或自签证书
限流 & JWT	连接限流、消息频率限制，Token 改 JWT 带过期时间
Docker 部署	Dockerfile + docker-compose.yml（server + redis + nats）
数据库管理接口	/backup 导出 SQLite，/clean 清理过期消息
建议顺序： Docker → TLS → 限流/JWT → Redis 在线状态 → NATS 路由 → Protobuf → DB 管理
v0.9+ 远景规划
方向	内容
多端客户端	Wails/Fyne 桌面 GUI，Web 端（Vue/React），移动端（Flutter）
高级搜索	全文检索（FTS5），按关键词/时间/联系人搜索历史消息
群组进阶	群管理员、群公告、@提及、群文件、群禁言
性能压测	万级并发连接测试，消息吞吐 benchmark，内存/GC 调优
插件/OpenAPI	机器人接口、Webhook、第三方扩展
测试 & Debug 策略
类型	覆盖目标
单元测试	service 层业务逻辑、localstore 文件操作
集成测试	多客户端 WebSocket 交互、消息路由、群广播
边界测试	好友上限、消息频率、大文件、长消息、并发登录
E2E 测试	注册→加好友→聊天→建群→离线→上线完整流程
兼容性	SQLite 函数兼容、Protobuf 向后兼容、Token 过期处理
建议下一步
1. 
v0.8: Docker 部署（Dockerfile + docker-compose.yml）
2. 
v0.8: TLS/SSL（WebSocket wss://）
3. 
v0.8: 限流 & JWT