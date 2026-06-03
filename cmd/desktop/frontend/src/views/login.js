import {api} from '../api.js';

export function renderLogin() {
  const app = document.getElementById('app');
  app.innerHTML = `
    <div class="login-page">
      <div class="login-card">
        <h1>QQGO</h1>
        <div class="tabs">
          <div class="tab active" data-tab="login">登录</div>
          <div class="tab" data-tab="register">注册</div>
        </div>
        <div id="login-form">
          <input type="number" id="login-qq" placeholder="QQ号">
          <input type="password" id="login-password" placeholder="密码">
          <button id="login-btn">登录</button>
        </div>
        <div id="register-form" style="display:none">
          <input type="text" id="reg-nickname" placeholder="昵称">
          <input type="password" id="reg-password" placeholder="密码">
          <button id="reg-btn">注册</button>
        </div>
        <div class="server-addr">
          <label>服务端地址</label>
          <input type="text" id="server-addr" value="ws://localhost:8080/ws">
        </div>
      </div>
    </div>
  `;

  document.querySelectorAll('.tab').forEach(tab => {
    tab.addEventListener('click', () => {
      document.querySelectorAll('.tab').forEach(t => t.classList.remove('active'));
      tab.classList.add('active');
      const isLogin = tab.dataset.tab === 'login';
      document.getElementById('login-form').style.display = isLogin ? 'block' : 'none';
      document.getElementById('register-form').style.display = isLogin ? 'none' : 'block';
    });
  });

  document.getElementById('login-btn').addEventListener('click', async () => {
    const addr = document.getElementById('server-addr').value;
    const qq = parseInt(document.getElementById('login-qq').value);
    const password = document.getElementById('login-password').value;
    if (!qq || !password) { alert('请输入 QQ 号和密码'); return; }
    window.store.set('serverAddr', addr);
    window.store.set('pendingLogin', {qq, password});
    try {
      await api.connect(addr);
      await api.login(qq, password);
    } catch (err) { alert('连接失败: ' + err); }
  });

  document.getElementById('reg-btn').addEventListener('click', async () => {
    const addr = document.getElementById('server-addr').value;
    const nickname = document.getElementById('reg-nickname').value;
    const password = document.getElementById('reg-password').value;
    if (!nickname || !password) { alert('请输入昵称和密码'); return; }
    window.store.set('serverAddr', addr);
    window.store.set('pendingRegister', {nickname, password});
    try {
      await api.connect(addr);
      await api.register(nickname, password);
    } catch (err) { alert('连接失败: ' + err); }
  });

  api.onLoginSuccess((data) => {
    const pending = window.store.get('pendingLogin');
    window.store.set('currentUser', {qq: data.qq, nickname: data.nickname, password: pending ? pending.password : ''});
    window.store.set('connected', true);
    window.store.set('pendingLogin', null);
    window.showChat();
  });

  api.onLoginFailed((data) => { alert('登录失败: ' + data.message); });

  api.onRegisterSuccess((data) => {
    const pending = window.store.get('pendingRegister');
    window.store.set('pendingRegister', null);
    if (pending) {
      window.store.set('pendingLogin', {qq: data.qq, password: pending.password});
      api.login(data.qq, pending.password);
    } else {
      alert('注册成功！QQ号: ' + data.qq + '\n请切换到登录页登录');
    }
  });

  api.onRegisterFailed((data) => { alert('注册失败: ' + data.message); });
}
