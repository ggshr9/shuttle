// gui/web/src/app/boot.ts
import { setAdapter } from '@/lib/data'
import { HttpAdapter } from '@/lib/data/http-adapter'
// BridgeAdapter import added in Task 5.5

export async function boot(): Promise<void> {
  // Phase 1 only — HTTP adapter for all runtimes.
  // Task 5.5 will replace this with bridge probe + fallback wiring.
  setAdapter(new HttpAdapter())
}
