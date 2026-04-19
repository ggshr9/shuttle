// Barrel — keeps `import { api } from './api'` working for legacy pages.
// New code should import from '@/lib/api/endpoints' or '@/lib/api/types'.

export * from './api/client'
export * from './api/types'
export * from './api/endpoints'
