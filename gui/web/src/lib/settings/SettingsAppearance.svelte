<script lang="ts">
  import { t, getLocale, setLocale, getLocales } from '../i18n/index'
  import { getTheme, setTheme, type Theme } from '../theme'

  let selectedLocale = $state(getLocale())
  let availableLocales = getLocales()
  let selectedTheme = $state(getTheme())

  function changeLocale(e) {
    const locale = e.target.value
    setLocale(locale)
    selectedLocale = locale
  }

  function changeTheme(e) {
    const theme = e.target.value as Theme
    setTheme(theme)
    selectedTheme = theme
  }
</script>

<section>
  <h3>{t('settings.language')}</h3>
  <label class="row">
    <span>{t('settings.language')}</span>
    <select value={selectedLocale} onchange={changeLocale}>
      {#each availableLocales as locale}
        <option value={locale.code}>{locale.name}</option>
      {/each}
    </select>
  </label>
  <label class="row">
    <span>{t('settings.theme')}</span>
    <select value={selectedTheme} onchange={changeTheme}>
      <option value="dark">{t('settings.dark')}</option>
      <option value="light">{t('settings.light')}</option>
    </select>
  </label>
</section>

<style>
  section {
    background: var(--bg-secondary);
    border: 1px solid var(--border);
    border-radius: 8px;
    padding: 12px 16px;
    margin-bottom: 12px;
  }

  h3 { font-size: 14px; color: var(--text-secondary); margin: 20px 0 10px; }

  .row {
    display: flex;
    align-items: center;
    gap: 8px;
    margin: 6px 0;
  }

  .row span { font-size: 13px; color: var(--text-secondary); min-width: 100px; }

  select {
    background: var(--bg-surface);
    border: 1px solid var(--border);
    border-radius: 6px;
    padding: 6px 10px;
    color: var(--text-primary);
    font-size: 13px;
  }
</style>
