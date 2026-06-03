export function renderSidebar() {
  const user = window.store.get('currentUser');
  const initial = user.nickname ? user.nickname[0].toUpperCase() : 'U';
  return `
    <div class="sidebar">
      <div class="avatar">${initial}</div>
      <div class="nav-item active" data-view="sessions">💬</div>
      <div class="nav-item" data-view="friends">👤</div>
      <div class="spacer"></div>
      <div class="nav-item" data-view="settings">⚙</div>
    </div>
  `;
}
