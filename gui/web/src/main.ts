/// <reference types="vite/client" />
import './app.css'
import { mount } from 'svelte'
import App from './App.svelte'

const target = document.getElementById('app')!

const params = typeof location !== 'undefined' ? new URLSearchParams(location.search) : null
if (import.meta.env.DEV && params?.get('ui') === '1') {
  // Dev-only UI primitive preview. Gated by env + query string so the
  // harness chunk is tree-shaken out of production builds.
  import('./__ui__/UIPreview.svelte').then((mod) => {
    mount(mod.default, { target })
  })
} else {
  mount(App, { target })
}
