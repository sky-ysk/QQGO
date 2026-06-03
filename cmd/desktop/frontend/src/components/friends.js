import {api} from '../api.js';

export function renderFriends() {
  const friends = window.store.get('friends') || [];
  const searchResults = window.store.get('searchResults') || [];
  const isSearching = searchResults.length > 0 || window.store.get('_searchActive');

  const groups = {};
  friends.forEach(f => {
    const g = f.groupName || '我的好友';
    if (!groups[g]) groups[g] = [];
    groups[g].push(f);
  });

  let friendHtml = '';
  if (!isSearching) {
    for (const [groupName, members] of Object.entries(groups)) {
      friendHtml += `<div class="friend-group-name">${groupName} (${members.length})</div>`;
      friendHtml += members.map(f => {
        const color = stringToColor(f.nickname || '');
        const initial = f.nickname ? f.nickname[0] : '?';
        const statusDot = f.online ? '<span class="status-dot online"></span>' : '';
        return `
          <div class="friend-item" data-qq="${f.qqNumber}">
            <div class="friend-avatar" style="background:${color}">${initial}${statusDot}</div>
            <div class="friend-info">
              <div class="friend-name">${f.nickname || f.qqNumber}</div>
              <div class="friend-qq">${f.qqNumber}</div>
            </div>
          </div>
        `;
      }).join('');
    }
    if (!friendHtml) friendHtml = '<div class="empty-state">暂无好友</div>';
  } else {
    friendHtml = searchResults.map(r => {
      const color = stringToColor(r.nickname || '');
      const initial = r.nickname ? r.nickname[0] : '?';
      const statusDot = r.online ? '<span class="status-dot online"></span>' : '';
      return `
        <div class="friend-item search-result" data-qq="${r.qqNumber}">
          <div class="friend-avatar" style="background:${color}">${initial}${statusDot}</div>
          <div class="friend-info">
            <div class="friend-name">${r.nickname || r.qqNumber}</div>
            <div class="friend-qq">${r.qqNumber}</div>
          </div>
        </div>
      `;
    }).join('');
    if (!friendHtml) friendHtml = '<div class="empty-state">未找到用户</div>';
  }

  return `
    <div class="session-list">
      <div class="search-bar">
        <input type="text" id="friend-search-input" placeholder="🔍 搜索好友或用户">
      </div>
      <div class="items">${friendHtml}</div>
    </div>
  `;
}

export function bindFriendEvents() {
  const searchInput = document.getElementById('friend-search-input');
  if (searchInput) {
    let debounceTimer;
    searchInput.addEventListener('input', () => {
      clearTimeout(debounceTimer);
      const keyword = searchInput.value.trim();
      if (!keyword) {
        window.store.set('searchResults', []);
        window.store.set('_searchActive', false);
        return;
      }
      debounceTimer = setTimeout(() => {
        window.store.set('_searchActive', true);
        api.searchUsers(keyword);
      }, 300);
    });
  }

  document.querySelectorAll('.friend-item').forEach(item => {
    item.addEventListener('click', () => {
      const qq = parseInt(item.dataset.qq);
      if (!qq) return;
      const friends = window.store.get('friends') || [];
      const friend = friends.find(f => f.qqNumber === qq);
      window.store.set('activeView', 'sessions');
      window.store.set('currentSession', {
        type: 'private',
        targetQQ: qq,
        nickname: friend ? friend.nickname : String(qq),
      });
      window.store.set('messages', []);
      window.store.set('searchResults', []);
      window.store.set('_searchActive', false);
      window.renderChatView();
      api.getHistory(qq, 0, 30);
    });
  });
}

function stringToColor(str) {
  const colors = ['#7BC67E', '#E8915A', '#6C5CE7', '#FD79A8', '#00B894', '#FDCB6E'];
  let hash = 0;
  for (let i = 0; i < str.length; i++) { hash = str.charCodeAt(i) + ((hash << 5) - hash); }
  return colors[Math.abs(hash) % colors.length];
}
