# QQGO 桌面端开发计划

**版本：** v0.4-desktop
**架构：** 前后端分离 — Electron 壳 + 纯 React 前端
**技术栈：** Electron + React + TypeScript + TailwindCSS + Zustand
**UI 风格：** 仿 QQ（深色侧边栏 + 浅色聊天区）
**日期：** 2026-05-22

---

## 架构概述

前后端完全分离。前端是纯 React Web 应用，通过 WebSocket 直连 QQGO 服务端。Electron 壳仅负责系统托盘和桌面通知。

```
┌─────────────────────────────────────────────────────────┐
│                    Electron 桌面应用                      │
│  ┌───────────────────────────────────────────────────┐  │
│  │  React 前端（渲染进程）                              │  │
│  │  ┌─────────┐  ┌──────────┐  ┌─────────────────┐  │  │
│  │  │ 登录页面 │  │ 聊天界面 │  │ 好友管理         │  │  │
│  │  └─────────┘  └──────────┘  └─────────────────┘  │  │
│  │  ┌─────────────────────────────────────────────┐  │  │
│  │  │ Zustand Store (auth/chat/friend)            │  │  │
│  │  └─────────────────────────────────────────────┘  │  │
│  │  ┌─────────────────────────────────────────────┐  │  │
│  │  │ WebSocket Client → ws://server:port/ws      │  │  │
│  │  └─────────────────────────────────────────────┘  │  │
│  └───────────────────────────────────────────────────┘  │
│  ┌───────────────────────────────────────────────────┐  │
│  │  Electron 主进程                                    │  │
│  │  - 窗口管理（创建/显示/隐藏）                        │  │
│  │  - 系统托盘（图标 + 菜单）                           │  │
│  │  - 桌面通知（收到消息时弹出）                         │  │
│  └───────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────┘
                          ↕ WebSocket
┌─────────────────────────────────────────────────────────┐
│  QQGO 服务端（Go, ws://localhost:8080/ws）               │
└─────────────────────────────────────────────────────────┘
```

### 关键优势
- **前端独立开发：** `cd frontend && npm run dev` 直接在浏览器调试
- **代码复用：** 前端 store/hooks 可直接复用到 React Native 移动端
- **Electron 轻量：** 主进程只负责托盘和通知，不包含业务逻辑
- **服务端零改动：** 前端直连现有 WebSocket 服务端

---

## 项目结构

```
QQGO/
├── desktop/                            # Electron 桌面端（独立项目）
│   ├── frontend/                       # React 前端（可独立运行）
│   │   ├── package.json
│   │   ├── vite.config.ts              # Vite 构建配置
│   │   ├── tailwind.config.js
│   │   ├── postcss.config.js
│   │   ├── .env.development            # WS_URL=ws://localhost:8080/ws
│   │   ├── .env.production             # WS_URL 可配置
│   │   ├── index.html
│   │   └── src/
│   │       ├── main.tsx                # React 入口
│   │       ├── App.tsx                 # 路由（登录/主界面）
│   │       ├── components/
│   │       │   ├── LoginPage.tsx           # 登录/注册页
│   │       │   ├── MainLayout.tsx          # 主布局（侧边栏+聊天区）
│   │       │   ├── Sidebar.tsx             # 左侧栏（用户信息+会话列表）
│   │       │   ├── ChatWindow.tsx          # 聊天窗口
│   │       │   ├── MessageList.tsx         # 消息列表（虚拟滚动）
│   │       │   ├── MessageBubble.tsx       # 消息气泡
│   │       │   ├── MessageInput.tsx        # 消息输入框
│   │       │   ├── FriendList.tsx          # 好友列表
│   │       │   ├── FriendItem.tsx          # 好友列表项
│   │       │   ├── SearchPanel.tsx         # 搜索面板
│   │       │   └── AddFriendModal.tsx      # 添加好友弹窗
│   │       ├── hooks/
│   │       │   ├── useWebSocket.ts         # WebSocket 连接/重连
│   │       │   ├── useMessageHandler.ts    # 消息分发处理
│   │       │   └── useOnlineStatus.ts      # 在线状态
│   │       ├── store/
│   │       │   ├── authStore.ts            # 登录状态（qq/nickname/token）
│   │       │   ├── chatStore.ts            # 聊天状态（会话/消息）
│   │       │   └── friendStore.ts          # 好友数据（列表/分组/搜索）
│   │       ├── types/
│   │       │   └── protocol.ts             # 消息协议（与 Go 端一致）
│   │       └── utils/
│   │           └── websocket.ts            # WebSocket 封装
│   │
│   ├── electron/                       # Electron 主进程
│   │   ├── package.json
│   │   ├── main.ts                     # 主进程入口
│   │   ├── preload.ts                  # 预加载脚本（安全通信）
│   │   ├── tray.ts                     # 系统托盘
│   │   ├── notification.ts             # 桌面通知
│   │   └── window.ts                   # 窗口管理
│   │
│   └── build/                          # 打包输出
│       └── QQGO-Desktop.app            # macOS 应用
│
├── cmd/                                # 服务端 + CLI（不变）
├── internal/                           # 服务端代码（不变）
└── ...
```

---

## 技术栈详情

| 层级 | 技术 | 说明 |
|------|------|------|
| 前端构建 | Vite 5 | 快速 HMR，TypeScript 原生支持 |
| 前端框架 | React 18 + TypeScript | 组件化，类型安全 |
| 样式 | TailwindCSS 3 | 原子化 CSS，QQ 风格配色 |
| 状态管理 | Zustand 4 | 轻量，无 boilerplate |
| WebSocket | 原生 WebSocket API | 浏览器原生，无需额外依赖 |
| 桌面框架 | Electron 30 | 成熟的桌面应用框架 |
| 打包 | electron-builder | 生成 .app / .exe / .deb |
| 虚拟滚动 | react-window | 大量消息性能优化 |

---

## 消息协议（TypeScript 定义）

前端需要维护与 Go 服务端一致的协议定义：

```typescript
// frontend/src/types/protocol.ts

export type MessageType = 
  | 1    // Text
  | 100  // Heartbeat
  | 101  // Login
  | 102  // LoginAck
  | 105  // Register
  | 106  // RegisterAck
  | 107  // ServerAck
  | 108  // Delivered
  | 300  // FriendRequest
  | 301  // FriendAccept
  | 302  // FriendReject
  | 303  // FriendDelete
  | 304  // FriendList
  | 305  // FriendSearch
  | 306  // FriendMoveGroup
  | 307  // FriendRemark
  | 308  // FriendGroups
  | 309  // FriendCreateGroup
  | 310  // FriendDeleteGroup
  | 311; // CheckUser

export interface Message {
  id: number;
  client_seq?: number;
  msg_type: MessageType;
  from_qq: number;
  to_qq: number;
  group_id?: string;
  content: string;
  delivered: boolean;
  created_at: string;
}

export interface LoginRequest {
  qq: number;
  password?: string;
  token?: string;
  platform: string;
}

export interface LoginResponse {
  code: number;
  message: string;
  token?: string;
  online: number;
  qq_number?: number;
  nickname?: string;
}

export interface RegisterRequest {
  password: string;
  nickname: string;
}

export interface RegisterResponse {
  code: number;
  message: string;
  qq_number?: number;
}

export interface FriendInfo {
  qq_number: number;
  nickname: string;
  remark?: string;
  group_name: string;
  status: number;
  online: boolean;
}

export interface FriendListResponse {
  friends: FriendInfo[];
  all_groups: string[];
}

export interface UserSearchResult {
  qq_number: number;
  nickname: string;
  online: boolean;
}

export interface CheckUserResponse {
  code: number;
  message: string;
  qq_number?: number;
  nickname?: string;
  online: boolean;
}
```

---

## 开发步骤

### Phase 1：前端项目初始化（0.5 天）

| 步骤 | 操作 | 验证 |
|------|------|------|
| 1.1 | `mkdir -p QQGO/desktop/frontend && cd QQGO/desktop/frontend` | 目录创建 |
| 1.2 | `npm create vite@latest . -- --template react-ts` | Vite 项目初始化 |
| 1.3 | `npm install zustand react-window` | 依赖安装 |
| 1.4 | `npm install -D tailwindcss postcss autoprefixer && npx tailwindcss init -p` | TailwindCSS 配置 |
| 1.5 | 配置 `tailwind.config.js` content 路径 + QQ 配色 | `npm run dev` 启动 |
| 1.6 | 创建 `src/types/protocol.ts`（消息协议定义） | TypeScript 编译通过 |
| 1.7 | 创建 `.env.development`（`VITE_WS_URL=ws://localhost:8080/ws`） | 环境变量可用 |
| 1.8 | 创建目录结构（components/ hooks/ store/ utils/） | 目录就绪 |

### Phase 2：前端核心 — WebSocket + Store（1 天）

| 步骤 | 操作 | 验证 |
|------|------|------|
| 2.1 | `utils/websocket.ts`：WebSocket 封装（connect/disconnect/send/onMessage） | 能连接服务端 |
| 2.2 | `utils/websocket.ts`：自动重连（断线 3s 重试，最多 5 次） | 服务端重启后重连 |
| 2.3 | `utils/websocket.ts`：心跳保活（30s Ping） | 连接不超时 |
| 2.4 | `store/authStore.ts`：登录状态（qq/nickname/token/online/connect） | Zustand store 正常 |
| 2.5 | `store/chatStore.ts`：聊天状态（currentQQ/messages/conversations） | 消息追加正常 |
| 2.6 | `store/friendStore.ts`：好友数据（friends/groups/searchResults） | 好友列表正常 |
| 2.7 | `hooks/useWebSocket.ts`：连接管理 + 消息分发到各 store | LoginAck 触发 authStore |
| 2.8 | `hooks/useMessageHandler.ts`：按 msg_type 分发处理 | 各类型消息正确处理 |

**WebSocket 核心逻辑：**
```typescript
// utils/websocket.ts
class QQGOWebSocket {
  private ws: WebSocket | null = null;
  private reconnectTimer: NodeJS.Timeout | null = null;
  private heartbeatTimer: NodeJS.Timeout | null = null;
  private reconnectAttempts = 0;
  private maxReconnectAttempts = 5;
  private reconnectDelay = 3000;

  constructor(private url: string, private onMessage: (data: Message) => void) {}

  connect() {
    this.ws = new WebSocket(this.url);
    this.ws.onopen = () => { this.startHeartbeat(); this.reconnectAttempts = 0; };
    this.ws.onmessage = (e) => { const msg = JSON.parse(e.data); this.onMessage(msg); };
    this.ws.onclose = () => { this.stopHeartbeat(); this.reconnect(); };
  }

  send(msg: Message) {
    if (this.ws?.readyState === WebSocket.OPEN) {
      this.ws.send(JSON.stringify(msg));
    }
  }

  login(qq: number, password: string) {
    this.send({ msg_type: 101, from_qq: 0, to_qq: 0, content: JSON.stringify({ qq, password, platform: "desktop" }), delivered: false, created_at: "" });
  }

  register(nickname: string, password: string) {
    this.send({ msg_type: 105, from_qq: 0, to_qq: 0, content: JSON.stringify({ nickname, password }), delivered: false, created_at: "" });
  }

  sendMessage(toQQ: number, content: string, clientSeq: number) {
    this.send({ msg_type: 1, client_seq: clientSeq, from_qq: 0, to_qq: toQQ, content, delivered: false, created_at: "" });
  }
}
```

### Phase 3：前端 UI — 登录 + 聊天 MVP（1.5 天）

| 步骤 | 操作 | 验证 |
|------|------|------|
| 3.1 | `LoginPage.tsx`：登录表单（QQ号 + 密码） | 表单提交调用 login() |
| 3.2 | `LoginPage.tsx`：注册表单（昵称 + 密码） | 注册成功自动登录 |
| 3.3 | `LoginPage.tsx`：错误提示 + 加载状态 | 用户体验良好 |
| 3.4 | `App.tsx`：路由逻辑（未登录→LoginPage，已登录→MainLayout） | 页面切换正常 |
| 3.5 | `MainLayout.tsx`：QQ 风格布局（深色侧边栏 250px + 浅色内容区） | 布局正确 |
| 3.6 | `Sidebar.tsx`：顶部用户信息（头像占位 + nickname(QQ)） | 显示登录信息 |
| 3.7 | `Sidebar.tsx`：会话列表（最近联系人，点击切换） | 列表交互正常 |
| 3.8 | `ChatWindow.tsx`：聊天头部（对方昵称 + QQ + 在线状态 ●/○） | 头部信息正确 |
| 3.9 | `MessageList.tsx`：消息列表（react-window 虚拟滚动） | 滚动流畅 |
| 3.10 | `MessageBubble.tsx`：QQ 风格气泡（我→右绿，对方→左白） | 样式正确 |
| 3.11 | `MessageBubble.tsx`：时间戳显示（HH:mm） | 时间格式正确 |
| 3.12 | `MessageInput.tsx`：输入框 + 发送按钮，Enter 发送 | 发送成功 |
| 3.13 | `MessageInput.tsx`：发送中状态（按钮禁用） | 状态正确 |
| 3.14 | 端到端测试：浏览器 `npm run dev` → 登录 → 与 CLI 客户端互发消息 | 完整流程通过 |

**QQ 风格配色：**
```javascript
// tailwind.config.js
colors: {
  'qq-dark': '#2e2e2e',
  'qq-dark-hover': '#3a3a3a',
  'qq-blue': '#12b7f5',
  'qq-green': '#95ec69',
  'qq-white': '#f5f5f5',
  'qq-bubble': '#ffffff',
  'qq-border': '#e0e0e0',
  'qq-text': '#333333',
  'qq-text-light': '#999999',
}
```

### Phase 4：前端 — 好友管理（1 天）

| 步骤 | 操作 | 验证 |
|------|------|------|
| 4.1 | `Sidebar.tsx`：好友/会话切换标签 | 标签切换正常 |
| 4.2 | `FriendList.tsx`：好友列表（分组折叠显示） | 分组正确 |
| 4.3 | `FriendItem.tsx`：好友项（头像 + 备注/昵称 + ●/○ + 右键菜单） | 样式+交互正确 |
| 4.4 | `FriendItem.tsx`：点击切换会话 | 切换到聊天窗口 |
| 4.5 | `SearchPanel.tsx`：搜索框（QQ号精确 + 昵称模糊） | 搜索结果正确 |
| 4.6 | `AddFriendModal.tsx`：添加好友（输入 QQ + 验证消息） | 请求发送成功 |
| 4.7 | 好友请求通知：收到 FriendRequest → 弹窗 → 接受/拒绝 | 交互完整 |
| 4.8 | 备注功能：右键好友 → 设置备注 | 备注更新 |
| 4.9 | `/to` 前置校验：切换会话前 CheckUser | 不存在的用户无法进入 |

### Phase 5：前端 — 分组管理 + 完善（0.5 天）

| 步骤 | 操作 | 验证 |
|------|------|------|
| 5.1 | 分组管理：创建/删除分组、移动好友 | 操作成功 |
| 5.2 | 空分组显示：所有分组都显示（包括空的） | 空分组可见 |
| 5.3 | 退出登录：清除状态 + 断开 WebSocket | 退出正常 |
| 5.4 | 错误处理：网络断开提示、服务端不可达提示 | 提示友好 |
| 5.5 | 加载状态：登录中、发送中、拉取好友中 | 状态反馈良好 |

### Phase 6：Electron 壳 — 托盘 + 通知（1 天）

| 步骤 | 操作 | 验证 |
|------|------|------|
| 6.1 | `mkdir QQGO/desktop/electron && cd electron && npm init -y` | 初始化 |
| 6.2 | `npm install electron electron-builder` | 依赖安装 |
| 6.3 | `main.ts`：Electron 主进程入口，创建 BrowserWindow | 窗口显示前端 |
| 6.4 | `main.ts`：加载前端构建产物（或开发模式加载 Vite dev server） | 前后端联通 |
| 6.5 | `preload.ts`：预加载脚本（安全 IPC 通信） | preload 正常 |
| 6.6 | `window.ts`：窗口管理（创建/显示/隐藏/关闭到托盘） | 关闭不退出 |
| 6.7 | `tray.ts`：系统托盘（图标 + 菜单：显示/退出） | 托盘正常 |
| 6.8 | `notification.ts`：桌面通知（收到消息时弹出） | 通知弹出 |
| 6.9 | 配置 `electron-builder`（macOS .app 打包） | `npm run build` 成功 |
| 6.10 | 端到端测试：`npm start` → 完整桌面应用 | 应用正常运行 |

**Electron 主进程核心：**
```typescript
// electron/main.ts
import { app, BrowserWindow, Tray, Menu, nativeImage } from 'electron';
import path from 'path';

let mainWindow: BrowserWindow | null = null;
let tray: Tray | null = null;

function createWindow() {
  mainWindow = new BrowserWindow({
    width: 900,
    height: 650,
    minWidth: 800,
    minHeight: 600,
    webPreferences: {
      preload: path.join(__dirname, 'preload.js'),
      contextIsolation: true,
      nodeIntegration: false,
    },
  });

  // 开发模式加载 Vite dev server，生产模式加载构建产物
  if (process.env.NODE_ENV === 'development') {
    mainWindow.loadURL('http://localhost:5173');
  } else {
    mainWindow.loadFile(path.join(__dirname, '../frontend/dist/index.html'));
  }

  // 关闭时隐藏到托盘，不退出
  mainWindow.on('close', (e) => {
    e.preventDefault();
    mainWindow?.hide();
  });
}

function createTray() {
  const icon = nativeImage.createFromPath(path.join(__dirname, '../assets/icon.png'));
  tray = new Tray(icon);
  const contextMenu = Menu.buildFromTemplate([
    { label: '显示', click: () => mainWindow?.show() },
    { type: 'separator' },
    { label: '退出', click: () => { app.isQuitting = true; app.quit(); } },
  ]);
  tray.setContextMenu(contextMenu);
  tray.on('click', () => mainWindow?.show());
}

app.whenReady().then(() => {
  createWindow();
  createTray();
});
```

### Phase 7：打包与优化（0.5 天）

| 步骤 | 操作 | 验证 |
|------|------|------|
| 7.1 | 应用图标：设计 QQGO 图标（.icns for macOS, .ico for Windows） | 图标正确 |
| 7.2 | `electron-builder` 配置：appId、productName、图标 | 配置正确 |
| 7.3 | `npm run build` 打包 macOS .app | 生成 .app |
| 7.4 | 性能优化：消息列表 react-window 虚拟滚动 | 大量消息流畅 |
| 7.5 | 生产环境配置：`.env.production` 支持配置服务端地址 | 地址可配置 |
| 7.6 | 首次启动引导：输入服务端地址（如果非默认） | 引导友好 |

---

## 预估时间线

| Phase | 内容 | 时间 | 累计 |
|-------|------|------|------|
| 1 | 前端项目初始化 | 0.5 天 | 0.5 天 |
| 2 | 前端核心（WebSocket + Store） | 1 天 | 1.5 天 |
| 3 | 前端 UI MVP（登录 + 聊天） | 1.5 天 | 3 天 |
| 4 | 前端好友管理 | 1 天 | 4 天 |
| 5 | 前端分组管理 + 完善 | 0.5 天 | 4.5 天 |
| 6 | Electron 壳（托盘 + 通知） | 1 天 | 5.5 天 |
| 7 | 打包与优化 | 0.5 天 | 6 天 |

**总计：约 6 天**

---

## 开发模式

```bash
# 终端 1：启动 QQGO 服务端
cd QQGO && go run ./cmd/server

# 终端 2：开发前端（浏览器）
cd QQGO/desktop/frontend && npm run dev
# → http://localhost:5173 浏览器访问，纯前端开发

# 终端 3：开发 Electron 壳（可选，需要时启动）
cd QQGO/desktop/electron && npm run dev
# → Electron 窗口加载 http://localhost:5173
```

**前端可完全在浏览器中开发和调试**，Electron 壳只在需要测试托盘/通知时启动。

---

## 服务端地址配置

```bash
# frontend/.env.development
VITE_WS_URL=ws://localhost:8080/ws

# frontend/.env.production
# 打包时可通过环境变量覆盖
VITE_WS_URL=ws://your-server.com:8080/ws
```

前端启动时读取 `import.meta.env.VITE_WS_URL`，支持：
- 开发环境：localhost
- 生产环境：环境变量或配置文件
- 用户自定义：设置界面输入服务端地址（存储在 localStorage）

---

## 移动端复用策略（React Native）

前端代码设计时考虑 React Native 复用：

| 可复用 | 不可复用（需重写） |
|--------|-------------------|
| `store/*`（Zustand 状态管理） | `components/*`（React DOM → RN 组件） |
| `hooks/useWebSocket.ts` | `tailwind.css`（→ RN StyleSheet） |
| `hooks/useMessageHandler.ts` | `index.html` / Vite 配置 |
| `types/protocol.ts` | Electron 壳 |
| `utils/websocket.ts` | — |

**移动端项目结构（后续）：**
```
mobile/                         # React Native 移动端
├── src/
│   ├── store/                  # 直接复制 desktop/frontend/src/store/
│   ├── hooks/                  # 直接复制 desktop/frontend/src/hooks/
│   ├── types/                  # 直接复制 desktop/frontend/src/types/
│   ├── utils/                  # 直接复制 desktop/frontend/src/utils/
│   └── components/             # RN 组件（重新实现）
│       ├── LoginScreen.tsx
│       ├── ChatScreen.tsx
│       └── ...
```

---

## 风险与注意事项

| 风险 | 应对 |
|------|------|
| WebSocket 断线重连时消息丢失 | 重连后自动拉取离线消息（服务端已有 pushOfflineMessages） |
| 大量消息渲染性能 | react-window 虚拟滚动，只渲染可见区域 |
| Electron 包体积（~150MB） | 可接受，后续考虑 electron-builder 优化 |
| macOS 系统托盘图标需要 @2x 尺寸 | 准备 16x16 和 32x32 两套图标 |
| 前端和 Electron 版本兼容 | 锁定 Electron 版本，定期更新 |
| TypeScript 类型与 Go 端不一致 | 手动维护 `protocol.ts`，每次改 Go 端同步更新 |

---

## 后续版本

**v0.5-desktop：**
- 会话历史记录（/to 后显示历史 + /prev /next 翻页）
- 群组聊天
- 会话列表（未读计数、最后消息摘要）
- 图片/文件消息

**v0.5-mobile（React Native）：**
- 复用 desktop 的 store/hooks/types/utils
- 重新实现移动端 UI 组件
- 移动端推送通知
