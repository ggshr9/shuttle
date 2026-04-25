/// <reference types="vite/client" />

// Dev-mode mock for the iOS/Android native bridge — activated via
// ?mockbridge=1 query param. No-op in production builds.
if (import.meta.env.DEV) {
  void import('./dev-bridge')
}

import './app.css'
import { mount } from 'svelte'
import App from './app/App.svelte'
import { boot } from './app/boot'

const target = document.getElementById('app')!
const params = typeof location !== 'undefined' ? new URLSearchParams(location.search) : null

void (async () => {
  await boot()
  if (import.meta.env.DEV && params?.get('ui') === '1') {
    const mod = await import('./__ui__/UIPreview.svelte')
    mount(mod.default, { target })
  } else {
    mount(App, { target })
  }
})()
