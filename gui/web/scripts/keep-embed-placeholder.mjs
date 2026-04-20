#!/usr/bin/env node
// Re-creates gui/web/dist/.gitkeep after a Vite build.
//
// Vite's `emptyOutDir: true` wipes everything in dist/ — including the
// tracked `.gitkeep` that keeps `//go:embed all:web/dist` happy on clean
// checkouts. Without this script, every local build leaves the working
// tree with `D gui/web/dist/.gitkeep`, which Codex flagged as a pre-merge
// hazard in review.
//
// Keep the text identical to the committed file so the working tree stays
// clean after `npm run build`.

import { writeFileSync } from 'node:fs'
import { resolve, dirname } from 'node:path'
import { fileURLToPath } from 'node:url'

const here = dirname(fileURLToPath(import.meta.url))
const target = resolve(here, '..', 'dist', '.gitkeep')

const CONTENT = `Placeholder so \`//go:embed all:web/dist\` has at least one file to match on
clean checkouts. The actual GUI assets are produced by \`npm run build\` in
gui/web/; release builds run that before \`go build\`. See CLAUDE.md.
`

writeFileSync(target, CONTENT)
console.log(`✓ preserved ${target}`)
