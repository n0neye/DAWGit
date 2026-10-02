import { mount } from 'svelte'
import App from './App.svelte'
import { startLanguage } from './lib/i18n.svelte'

// The language first: texts are shown in it from the start.
startLanguage().finally(() => {
  mount(App, { target: document.getElementById('app')! })
  // The app shows its own splash until it has read the teams.
  document.getElementById('splash')?.remove()
})
