/**
 * Mavzu (to'q / yorug'). Tanlov localStorage'da saqlanadi va
 * <html data-theme> atributiga qo'yiladi — CSS shunga qarab ranglarni
 * almashtiradi.
 */
const KEY = 'minidss_theme'

export function useTheme() {
  const theme = useState<'dark' | 'light'>('theme', () => 'dark')

  function apply(next: 'dark' | 'light') {
    theme.value = next
    if (import.meta.client) {
      document.documentElement.dataset.theme = next
      localStorage.setItem(KEY, next)
    }
  }

  function init() {
    if (!import.meta.client) return
    const saved = localStorage.getItem(KEY) as 'dark' | 'light' | null
    // Saqlangani bo'lmasa tizim sozlamasiga ergashamiz.
    const prefersLight = window.matchMedia?.('(prefers-color-scheme: light)').matches
    apply(saved ?? (prefersLight ? 'light' : 'dark'))
  }

  const toggle = () => apply(theme.value === 'dark' ? 'light' : 'dark')

  return { theme, init, toggle }
}
