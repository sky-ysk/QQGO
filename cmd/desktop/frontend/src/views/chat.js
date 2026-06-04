import {api} from '../api.js';
import {renderSidebar} from '../components/sidebar.js';
import {renderSessions} from '../components/sessions.js';
import {renderFriends, bindFriendEvents} from '../components/friends.js';
import {renderMessages, scrollToBottom} from '../components/messages.js';

let eventsBound = false;
const PAGE_SIZE = 30;

export function initChat() {
  renderChatView();
  if (!eventsBound) {
    eventsBound = true;
    registerEvents();
  }
  api.getSessions();
  api.getFriendList();
}

export function renderChatView() {
  const activeView = window.store.get('activeView') || 'sessions';
  const middlePanel = activeView === 'friends' ? renderFriends() : renderSessions();

  const app = document.getElementById('app');
  app.innerHTML = `
    <div class="main-window">
      ${renderSidebar()}
      ${middlePanel}
      ${renderMessages()}
    </div>
  `;
  bindSidebarEvents();
  if (activeView === 'friends') {
    bindFriendEvents();
  } else {
    bindChatEvents();
  }
  bindScrollLoadMore();
}

function bindSidebarEvents() {
  document.querySelectorAll('.sidebar .nav-item').forEach(item => {
    item.addEventListener('click', () => {
      const view = item.dataset.view;
      if (view === 'logout') {
        doLogout();
        return;
      }
      if (view === 'sessions' || view === 'friends') {
        window.store.set('activeView', view);
        if (view === 'friends') {
          api.getFriendList();
        }
        renderChatView();
      }
    });
  });
}

function doLogout() {
  window.store.set('loggedOut', true);
  api.disconnect();
  api.clearCredentials();
  window.store.set('currentUser', null);
  window.store.set('sessions', []);
  window.store.set('friends', []);
  window.store.set('currentSession', null);
  window.store.set('messages', []);
  window.store.set('connected', false);
  window.store.set('activeView', 'sessions');
  window.store.set('historyOffset', 0);
  window.store.set('historyHasMore', true);
  window.store.set('historyLoading', false);
  window.showLogin();
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
      window.store.set('historyOffset', 0);
      window.store.set('historyHasMore', true);
      window.store.set('historyLoading', true);
      renderChatView();
      if (type === 'private') {
        api.getHistory(targetQQ, 0, PAGE_SIZE);
      } else if (type === 'group') {
        api.getGroupHistory(groupID, 0, PAGE_SIZE);
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
      renderChatView();
      setTimeout(scrollToBottom, 50);
    };
    sendBtn.addEventListener('click', send);
    input.addEventListener('keydown', (e) => {
      if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); send(); }
    });
  }
}

function bindScrollLoadMore() {
  const container = document.getElementById('messages-container');
  if (!container) return;
  container.addEventListener('scroll', () => {
    if (container.scrollTop > 50) return;
    if (window.store.get('historyLoading')) return;
    if (!window.store.get('historyHasMore')) return;
    const current = window.store.get('currentSession');
    if (!current) return;
    const offset = window.store.get('historyOffset') || 0;
    window.store.set('historyLoading', true);
    if (current.type === 'private') {
      api.getHistory(current.targetQQ, offset, PAGE_SIZE);
    } else if (current.type === 'group') {
      api.getGroupHistory(current.groupID, offset, PAGE_SIZE);
    }
  });
}

function registerEvents() {
  api.onSessionsUpdated((sessions) => {
    window.store.set('sessions', sessions);
    if (document.querySelector('.main-window')) { renderChatView(); }
  });

  api.onFriendsLoaded((friends) => {
    window.store.set('friends', friends);
    if (document.querySelector('.main-window') && window.store.get('activeView') === 'friends') {
      renderChatView();
    }
  });

  api.onSearchResults((results) => {
    window.store.set('searchResults', results);
    if (document.querySelector('.main-window') && window.store.get('activeView') === 'friends') {
      renderChatView();
      const input = document.getElementById('friend-search-input');
      if (input) { input.focus(); input.setSelectionRange(input.value.length, input.value.length); }
    }
  });

  api.onHistoryLoaded((messages) => {
    const existing = window.store.get('messages') || [];
    const offset = window.store.get('historyOffset') || 0;

    if (offset === 0) {
      window.store.set('messages', messages);
      window.store.set('historyOffset', messages.length);
      window.store.set('historyHasMore', messages.length >= PAGE_SIZE);
      window.store.set('historyLoading', false);
      if (document.querySelector('.main-window')) { renderChatView(); setTimeout(scrollToBottom, 50); }
    } else {
      if (messages.length === 0) {
        window.store.set('historyHasMore', false);
        window.store.set('historyLoading', false);
        return;
      }
      const merged = [...messages, ...existing];
      window.store.set('messages', merged);
      window.store.set('historyOffset', offset + messages.length);
      window.store.set('historyHasMore', messages.length >= PAGE_SIZE);
      window.store.set('historyLoading', false);
      if (document.querySelector('.main-window')) {
        renderChatView();
        const container = document.getElementById('messages-container');
        if (container) {
          container.scrollTop = messages.length * 60;
        }
      }
    }
  });

  api.onMessageReceived((msg) => {
    const current = window.store.get('currentSession');
    const messages = window.store.get('messages') || [];
    if (current &&
        ((current.type === 'private' && (msg.fromQQ === current.targetQQ || msg.toQQ === current.targetQQ)) ||
         (current.type === 'group' && msg.groupID === current.groupID))) {
      messages.push(msg);
      window.store.set('messages', messages);
      if (document.querySelector('.main-window')) { renderChatView(); setTimeout(scrollToBottom, 50); }
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
  if (window.store.get('loggedOut')) {
    return;
  }
  if (reconnectAttempt >= maxReconnectAttempts) {
    showConnectionBanner('无法连接，请检查服务端');
    return;
  }
  const delays = [3000, 6000, 12000, 24000, 48000];
  const delay = delays[reconnectAttempt] || 48000;
  reconnectAttempt++;

  setTimeout(async () => {
    if (window.store.get('loggedOut')) {
      return;
    }
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
