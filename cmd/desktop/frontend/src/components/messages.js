export function renderMessages() {
  const current = window.store.get('currentSession');
  const messages = window.store.get('messages') || [];
  const currentUser = window.store.get('currentUser');
  const historyLoading = window.store.get('historyLoading');
  const historyHasMore = window.store.get('historyHasMore');

  if (!current) {
    return '<div class="chat-area"><div class="empty-state">选择一个会话开始聊天</div></div>';
  }

  const title = current.nickname || current.targetQQ || current.groupID;
  const subtitle = current.type === 'private' ? `<span class="qq">${current.targetQQ}</span>` : '';

  let topHint = '';
  if (historyLoading) {
    topHint = '<div class="history-hint">加载中...</div>';
  } else if (messages.length > 0 && !historyHasMore) {
    topHint = '<div class="history-hint">没有更多消息了</div>';
  }

  const msgHtml = messages.map(m => {
    const isSelf = m.fromQQ === currentUser.qq;
    const avatarColor = isSelf ? '#12B7F5' : '#7BC67E';
    const avatarText = isSelf ? (currentUser.nickname ? currentUser.nickname[0] : '我') : (title ? title[0] : '?');
    return `
      <div class="message ${isSelf ? 'self' : ''}">
        <div class="avatar" style="background:${avatarColor}">${avatarText}</div>
        <div class="bubble">${escapeHtml(m.content)}</div>
      </div>
    `;
  }).join('');

  return `
    <div class="chat-area">
      <div class="header"><div class="title">${title} ${subtitle}</div></div>
      <div class="messages" id="messages-container">${topHint}${msgHtml}</div>
      <div class="input-area">
        <div class="toolbar"><span>😊</span><span>📎</span><span>📷</span></div>
        <div class="input-row">
          <textarea id="message-input" placeholder="输入消息..."></textarea>
          <button class="send-btn" id="send-btn">发送</button>
        </div>
      </div>
    </div>
  `;
}

function escapeHtml(text) {
  const div = document.createElement('div');
  div.textContent = text;
  return div.innerHTML;
}

export function scrollToBottom() {
  const container = document.getElementById('messages-container');
  if (container) { container.scrollTop = container.scrollHeight; }
}
