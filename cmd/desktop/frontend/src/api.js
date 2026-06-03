import {Connect, Disconnect, Login, Register, SendMessage, SendGroupMessage, GetSessions, GetFriendList, GetHistory, GetGroupHistory, SearchUsers, SaveCredentials, LoadCredentials, ClearCredentials} from '../wailsjs/go/main/App.js';
import {EventsOn} from '../wailsjs/runtime/runtime.js';

export const api = {
  connect: (addr) => Connect(addr),
  disconnect: () => Disconnect(),
  login: (qq, password) => Login(qq, password),
  register: (nickname, password) => Register(nickname, password),
  sendMessage: (toQQ, content) => SendMessage(toQQ, content),
  sendGroupMessage: (groupID, content) => SendGroupMessage(groupID, content),
  getSessions: () => GetSessions(),
  getFriendList: () => GetFriendList(),
  getHistory: (targetQQ, offset, limit) => GetHistory(targetQQ, offset, limit),
  getGroupHistory: (groupID, offset, limit) => GetGroupHistory(groupID, offset, limit),
  searchUsers: (keyword) => SearchUsers(keyword),
  saveCredentials: (addr, qq, password) => SaveCredentials(addr, qq, password),
  loadCredentials: () => LoadCredentials(),
  clearCredentials: () => ClearCredentials(),

  onLoginSuccess: (cb) => EventsOn("login-success", cb),
  onLoginFailed: (cb) => EventsOn("login-failed", cb),
  onRegisterSuccess: (cb) => EventsOn("register-success", cb),
  onRegisterFailed: (cb) => EventsOn("register-failed", cb),
  onSessionsUpdated: (cb) => EventsOn("sessions-updated", cb),
  onFriendsLoaded: (cb) => EventsOn("friends-loaded", cb),
  onHistoryLoaded: (cb) => EventsOn("history-loaded", cb),
  onMessageReceived: (cb) => EventsOn("message-received", cb),
  onMessageAck: (cb) => EventsOn("message-ack", cb),
  onConnectionLost: (cb) => EventsOn("connection-lost", cb),
  onSearchResults: (cb) => EventsOn("search-results", cb),
};
