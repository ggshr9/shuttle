// Browser notification utility

let permission: NotificationPermission = 'default'

// Request notification permission
export async function requestPermission(): Promise<boolean> {
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
export function canNotify(): boolean {
  return 'Notification' in window && Notification.permission === 'granted'
}

export interface NotifyOptions {
  body?: string
  icon?: string
  badge?: string
  tag?: string
  silent?: boolean
  requireInteraction?: boolean
}

// Show a notification
export function notify(title: string, options: NotifyOptions = {}): Notification | null {
  if (!canNotify()) {
    return null
  }

  const defaultOptions: NotifyOptions = {
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
export function notifyConnected(serverName?: string): Notification | null {
  return notify('Shuttle Connected', {
    body: serverName ? `Connected to ${serverName}` : 'Proxy connection established',
    tag: 'shuttle-connection',
  })
}

export function notifyDisconnected(): Notification | null {
  return notify('Shuttle Disconnected', {
    body: 'Proxy connection closed',
    tag: 'shuttle-connection',
  })
}

export function notifyError(message: string): Notification | null {
  return notify('Shuttle Error', {
    body: message,
    tag: 'shuttle-error',
  })
}
