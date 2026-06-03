import './store.js';
import {renderLogin} from './views/login.js';
import {initChat} from './views/chat.js';

window.showChat = initChat;
renderLogin();
