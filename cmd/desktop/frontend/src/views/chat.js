import {api} from '../api.js';
import {renderSidebar} from '../components/sidebar.js';
import {renderSessions} from '../components/sessions.js';
import {renderMessages, scrollToBottom} from '../components/messages.js';

let eventsBound = false;

export function initChat() {
  renderChat();
  if (!eventsBound) {
    eventsBound = true;
    registerEvents();
  }
  api.getSessions();
  api.getFriendList();
}

export function renderChat() {
  const app = document.getElementById('app');
  app.innerHTML = `
    <div class="main-window">
      ${renderSidebar()}
      ${renderSessions()}
      ${renderMessages()}
    </div>
  `;
  bindChatEvents();
}

function bindChatEvents() {
  document.querySelectorAll('.session-item').forEach(item => {
    item.addEventListener('click', () => {
      const type = item.dataset.type;
      const targetQQ = parseInt(item.dataset.target) || 0;
      const groupID = item.dataset.group || '';
      const session = {type, targetQQ, groupID};
      const sessions = window.store.get('sessions') || [];
      const s = sessions.find(s =>
        (type === 'private' && s.targetQQ === targetQQ) ||
        (type === 'group' && s.groupID === groupID)
      );
      if (s) session.nickname = s.nickname;
      window.store.set('currentSession', session);
      window.store.set('messages', []);
      renderChat();
      if (type === 'private') {
        api.getHistory(targetQQ, 0, 30);
      } else if (type === 'group') {
        api.getGroupHistory(groupID, 0, 30);
      }
    });
  });

  const sendBtn = document.getElementById('send-btn');
  const input = document.getElementById('message-input');
  if (sendBtn && input) {
    const send = () => {
      const content = input.value.trim();
      if (!content) return;
      const current = window.store.get('currentSession');
      if (!current) return;
      if (current.type === 'private') { api.sendMessage(current.targetQQ, content); }
      else { api.sendGroupMessage(current.groupID, content); }
      const messages = window.store.get('messages') || [];
      messages.push({ fromQQ: window.store.get('currentUser').qq, content: content, createdAt: Math.floor(Date.now() / 1000) });
      window.store.set('messages', messages);
      input.value = '';
      renderChat();
      setTimeout(scrollToBottom, 50);
    };
    sendBtn.addEventListener('click', send);
    input.addEventListener('keydown', (e) => {
      if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); send(); }
    });
  }
}

function registerEvents() {
  api.onSessionsUpdated((sessions) => {
    window.store.set('sessions', sessions);
    if (document.querySelector('.main-window')) { renderChat(); }
  });

  api.onFriendsLoaded((friends) => { window.store.set('friends', friends); });

  api.onHistoryLoaded((messages) => {
    window.store.set('messages', messages);
    if (document.querySelector('.main-window')) { renderChat(); setTimeout(scrollToBottom, 50); }
  });

  api.onMessageReceived((msg) => {
    const current = window.store.get('currentSession');
    const messages = window.store.get('messages') || [];
    if (current &&
        ((current.type === 'private' && (msg.fromQQ === current.targetQQ || msg.toQQ === current.targetQQ)) ||
         (current.type === 'group' && msg.groupID === current.groupID))) {
      messages.push(msg);
      window.store.set('messages', messages);
      if (document.querySelector('.main-window')) { renderChat(); setTimeout(scrollToBottom, 50); }
    }
    api.getSessions();
  });

  api.onConnectionLost(() => {
    window.store.set('connected', false);
    showConnectionBanner('连接已断开，正在重连...');
    startReconnect();
  });
}

function showConnectionBanner(text) {
  let banner = document.querySelector('.connection-banner');
  if (!banner) {
    banner = document.createElement('div');
    banner.className = 'connection-banner';
    document.body.appendChild(banner);
  }
  banner.textContent = text;
}

function removeConnectionBanner() {
  const banner = document.querySelector('.connection-banner');
  if (banner) banner.remove();
}

let reconnectAttempt = 0;
const maxReconnectAttempts = 5;

function startReconnect() {
  if (reconnectAttempt >= maxReconnectAttempts) {
    showConnectionBanner('无法连接，请检查服务端');
    return;
  }
  const delays = [3000, 6000, 12000, 24000, 48000];
  const delay = delays[reconnectAttempt] || 48000;
  reconnectAttempt++;

  setTimeout(async () => {
    try {
      const addr = window.store.get('serverAddr') || 'ws://localhost:8080/ws';
      await api.connect(addr);
      const user = window.store.get('currentUser');
      if (user) {
        await api.login(user.qq, user.password || '');
      }
      reconnectAttempt = 0;
      removeConnectionBanner();
      window.store.set('connected', true);
    } catch (e) {
      startReconnect();
    }
  }, delay);
}
