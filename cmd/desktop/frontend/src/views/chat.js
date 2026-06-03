import {api} from '../api.js';
import {renderSidebar} from '../components/sidebar.js';
import {renderSessions} from '../components/sessions.js';
import {renderMessages, scrollToBottom} from '../components/messages.js';

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
  loadInitialData();
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
      if (type === 'private') { api.getHistory(targetQQ, 0, 30); }
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

function loadInitialData() {
  api.getSessions();
  api.getFriendList();
}

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
  const banner = document.createElement('div');
  banner.className = 'connection-banner';
  banner.textContent = '连接已断开，正在重连...';
  document.body.appendChild(banner);
});
