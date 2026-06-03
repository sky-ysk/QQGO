export function renderSessions() {
  const sessions = window.store.get('sessions') || [];
  const current = window.store.get('currentSession');

  const items = sessions.map(s => {
    const isActive = current &&
      ((s.type === 'private' && current.targetQQ === s.targetQQ) ||
       (s.type === 'group' && current.groupID === s.groupID));
    const avatarClass = s.type === 'group' ? 'avatar group' : 'avatar';
    const avatarText = s.type === 'group' ? '群' : (s.nickname ? s.nickname[0] : '?');
    const avatarColor = s.type === 'group' ? '#6C5CE7' : stringToColor(s.nickname || '');
    const time = formatTime(s.lastTime);
    return `
      <div class="session-item ${isActive ? 'active' : ''}"
           data-type="${s.type}" data-target="${s.targetQQ || ''}" data-group="${s.groupID || ''}">
        <div class="${avatarClass}" style="background:${avatarColor}">${avatarText}</div>
        <div class="info">
          <div class="top">
            <span class="name">${s.nickname || s.targetQQ || s.groupID}</span>
            <span class="time">${time}</span>
          </div>
          <div class="last-msg">${s.lastMessage || ''}</div>
        </div>
      </div>
    `;
  }).join('');

  return `
    <div class="session-list">
      <div class="search-bar"><input type="text" placeholder="🔍 搜索"></div>
      <div class="items">${items || '<div class="empty-state">暂无会话</div>'}</div>
    </div>
  `;
}

function stringToColor(str) {
  const colors = ['#7BC67E', '#E8915A', '#6C5CE7', '#FD79A8', '#00B894', '#FDCB6E'];
  let hash = 0;
  for (let i = 0; i < str.length; i++) { hash = str.charCodeAt(i) + ((hash << 5) - hash); }
  return colors[Math.abs(hash) % colors.length];
}

function formatTime(timestamp) {
  if (!timestamp) return '';
  const date = new Date(timestamp * 1000);
  const now = new Date();
  const diff = now - date;
  if (diff < 60000) return '刚刚';
  if (diff < 3600000) return Math.floor(diff / 60000) + '分钟前';
  if (diff < 86400000) return date.getHours() + ':' + String(date.getMinutes()).padStart(2, '0');
  if (diff < 172800000) return '昨天';
  return (date.getMonth() + 1) + '/' + date.getDate();
}
