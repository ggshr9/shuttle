import type { Server } from '@/lib/api/types'

export type AddMethod = 'subscription' | 'import' | 'manual'

export interface WizardState {
  step: 1 | 2 | 3 | 4
  method: AddMethod
  subscriptionUrl: string
  importData: string
  manualAddr: string
  manualPassword: string
  enableSystemProxy: boolean
  enableMesh: boolean
  meshAvailable: boolean
  addedServers: Server[]
  error: string
  busy: boolean
}

export function createWizard(): WizardState {
  const w = $state<WizardState>({
    step: 1,
    method: 'subscription',
    subscriptionUrl: '',
    importData: '',
    manualAddr: '',
    manualPassword: '',
    enableSystemProxy: true,
    enableMesh: true,
    meshAvailable: false,
    addedServers: [],
    error: '',
    busy: false,
  })
  return w
}
