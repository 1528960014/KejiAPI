const THEME_STORAGE = 'modelhub-theme'

export type Theme = 'dark' | 'light'

export function getTheme(): Theme {
  const stored = localStorage.getItem(THEME_STORAGE)
  return stored === 'light' ? 'light' : 'dark'
}

export function applyTheme(theme: Theme): void {
  document.documentElement.classList.toggle('dark', theme === 'dark')
  document.documentElement.dataset.theme = theme
  localStorage.setItem(THEME_STORAGE, theme)
}

export function toggleTheme(): Theme {
  const next: Theme = getTheme() === 'dark' ? 'light' : 'dark'
  applyTheme(next)
  return next
}