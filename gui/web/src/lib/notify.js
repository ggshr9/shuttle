// Browser notification utility

let permission = 'default'

// Request notification permission
export async function requestPermission() {
  if (!('Notification' in window)) {
    return false
  }

  if (Notification.permission === 'granted') {
    permission = 'granted'
    return true
  }

  if (Notification.permission !== 'denied') {
    const result = await Notification.requestPermission()
    permission = result
    return result === 'granted'
  }

  return false
}

// Check if notifications are supported and permitted
export function canNotify() {
  return 'Notification' in window && Notification.permission === 'granted'
}

// Show a notification
export function notify(title, options = {}) {
  if (!canNotify()) {
    return null
  }

  const defaultOptions = {
    icon: '/favicon.ico',
    badge: '/favicon.ico',
    silent: false,
    requireInteraction: false,
  }

  try {
    return new Notification(title, { ...defaultOptions, ...options })
  } catch (e) {
    console.warn('Notification failed:', e)
    return null
  }
}

// Connection state notifications
export function notifyConnected(serverName) {
  return notify('Shuttle Connected', {
    body: serverName ? `Connected to ${serverName}` : 'Proxy connection established',
    tag: 'shuttle-connection',
  })
}

export function notifyDisconnected() {
  return notify('Shuttle Disconnected', {
    body: 'Proxy connection closed',
    tag: 'shuttle-connection',
  })
}

export function notifyError(message) {
  return notify('Shuttle Error', {
    body: message,
    tag: 'shuttle-error',
  })
}
