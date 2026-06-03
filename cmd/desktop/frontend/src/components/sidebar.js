export function renderSidebar() {
  const user = window.store.get('currentUser');
  const initial = user.nickname ? user.nickname[0].toUpperCase() : 'U';
  const activeView = window.store.get('activeView') || 'sessions';
  return `
    <div class="sidebar">
      <div class="avatar">${initial}</div>
      <div class="nav-item ${activeView === 'sessions' ? 'active' : ''}" data-view="sessions">💬</div>
      <div class="nav-item ${activeView === 'friends' ? 'active' : ''}" data-view="friends">👤</div>
      <div class="spacer"></div>
      <div class="nav-item" data-view="logout" title="退出登录">⚙</div>
    </div>
  `;
}
