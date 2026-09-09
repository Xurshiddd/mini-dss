/**
 * API klienti — token bilan.
 *
 * Token localStorage'da saqlanadi. 401 kelsa token o'chiriladi va login
 * sahifasiga qaytariladi (masalan token muddati tugaganda).
 */

const TOKEN_KEY = 'minidss_token'
const USER_KEY = 'minidss_user'

export function useAuthToken() {
  return {
    get: () => (import.meta.client ? localStorage.getItem(TOKEN_KEY) : null),
    user: () => (import.meta.client ? localStorage.getItem(USER_KEY) : null),
    set: (token: string, user: string) => {
      localStorage.setItem(TOKEN_KEY, token)
      localStorage.setItem(USER_KEY, user)
    },
    clear: () => {
      localStorage.removeItem(TOKEN_KEY)
      localStorage.removeItem(USER_KEY)
    },
  }
}

export function useApi() {
  const config = useRuntimeConfig()
  const token = useAuthToken()
  const base = config.public.apiBase as string

  async function request<T>(path: string, options: any = {}): Promise<T> {
    const headers: Record<string, string> = { ...(options.headers || {}) }

    const jwt = token.get()
    if (jwt) headers.Authorization = `Bearer ${jwt}`

    try {
      return await $fetch<T>(base + path, { ...options, headers })
    } catch (err: any) {
      if (err?.response?.status === 401) {
        token.clear()
        await navigateTo('/login')
      }
      // API xatolarni {"error": "..."} shaklida qaytaradi.
      throw new Error(err?.data?.error || err?.message || 'Xato yuz berdi')
    }
  }

  /**
   * Himoyalangan faylni blob sifatida oladi va vaqtinchalik URL qaytaradi.
   *
   * ⚠️ Token URL'ga EMAS, Authorization sarlavhasiga qo'yiladi — shu sababli
   * u server loglariga, brauzer tarixiga yoki Referer'ga tushmaydi.
   * Chaqiruvchi URL'ni `revokePhoto` bilan bo'shatishi kerak.
   *
   * Fayl bo'lmasa (404) bo'sh satr qaytadi.
   */
  async function blobUrl(path: string): Promise<string> {
    const jwt = token.get()
    const res = await fetch(base + path, {
      headers: jwt ? { Authorization: `Bearer ${jwt}` } : {},
    })
    if (!res.ok) {
      if (res.status === 401) {
        token.clear()
        await navigateTo('/login')
      }
      return ''
    }
    return URL.createObjectURL(await res.blob())
  }

  return {
    base,
    get: <T>(path: string) => request<T>(path),
    post: <T>(path: string, body?: any) => request<T>(path, { method: 'POST', body }),
    put: <T>(path: string, body?: any) => request<T>(path, { method: 'PUT', body }),
    del: <T>(path: string) => request<T>(path, { method: 'DELETE' }),
    upload: <T>(path: string, form: FormData) =>
      request<T>(path, { method: 'POST', body: form }),
    /**
     * Fayl yuklab olish (XLSX va shu kabilar).
     *
     * ⚠️ Oddiy `<a href>` ishlamaydi — token sarlavhada ketishi kerak.
     * Shu sababli javob blob sifatida olinadi va vaqtinchalik havola
     * yaratiladi.
     */
    async download(path: string, filename: string): Promise<void> {
      const jwt = token.get()
      const res = await fetch(base + path, {
        headers: jwt ? { Authorization: `Bearer ${jwt}` } : {},
      })

      if (!res.ok) {
        if (res.status === 401) {
          token.clear()
          await navigateTo('/login')
        }
        throw new Error(`Yuklab olinmadi (HTTP ${res.status})`)
      }

      const url = URL.createObjectURL(await res.blob())
      const link = document.createElement('a')
      link.href = url
      link.download = filename
      link.click()
      URL.revokeObjectURL(url)
    },
    blobUrl,
    photoBlob: (personId: number) => blobUrl(`/api/people/${personId}/photo`),

    revokePhoto(url: string) {
      if (url) URL.revokeObjectURL(url)
    },
  }
}
