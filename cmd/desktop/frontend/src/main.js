import './store.js';
import {renderLogin} from './views/login.js';
import {initChat, renderChatView} from './views/chat.js';

window.showChat = initChat;
window.showLogin = renderLogin;
window.renderChatView = renderChatView;
renderLogin();
