// gui/web/src/lib/data/bridge-transport.ts

export interface BridgeEnvelope {
  method: string
  path: string
  headers: Record<string, string>
  body?: string                   // base64
}

export interface BridgeResponse {
  status: number                   // -1 for transport error
  headers: Record<string, string>
  body: string                     // base64
  error?: string | null
}

export interface ShuttleBridgeAPI {
  send(envelope: BridgeEnvelope): Promise<BridgeResponse>
}

declare global {
  interface Window {
    ShuttleBridge?: ShuttleBridgeAPI & {
      _complete?: (id: number, response: BridgeResponse) => void
      _fail?: (id: number, msg: string) => void
    }
  }
}

/** Caller-injectable transport function. Default: dispatches via window.ShuttleBridge.
 *  Throws when window.ShuttleBridge is missing — caller must verify availability first. */
export type BridgeSend = (envelope: BridgeEnvelope) => Promise<BridgeResponse>

export const bridgeSend: BridgeSend = (envelope) => {
  if (typeof window === 'undefined' || !window.ShuttleBridge) {
    throw new Error('window.ShuttleBridge not available — BridgeAdapter requires it')
  }
  return window.ShuttleBridge.send(envelope)
}
