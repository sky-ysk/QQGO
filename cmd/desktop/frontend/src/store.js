class Store {
  constructor() {
    this.state = {
      currentUser: null,
      sessions: [],
      friends: [],
      currentSession: null,
      messages: [],
      connected: false,
      activeView: 'sessions',
      searchResults: [],
      loggedOut: false,
    };
    this.listeners = {};
  }

  get(key) {
    return this.state[key];
  }

  set(key, value) {
    this.state[key] = value;
    this.emit(key, value);
  }

  on(event, callback) {
    if (!this.listeners[event]) {
      this.listeners[event] = [];
    }
    this.listeners[event].push(callback);
  }

  emit(event, data) {
    if (this.listeners[event]) {
      this.listeners[event].forEach(cb => cb(data));
    }
  }
}

window.store = new Store();
