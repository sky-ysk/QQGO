import './store.js';
import {renderLogin} from './views/login.js';
import {renderChat} from './views/chat.js';

window.showChat = renderChat;
renderLogin();
