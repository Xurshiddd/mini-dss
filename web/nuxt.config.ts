export default defineNuxtConfig({
  compatibilityDate: '2026-08-01',
  ssr: false,               // admin panel — SEO kerak emas, SPA yetarli
  devtools: { enabled: false },
  css: ['~/assets/css/main.css'],
  runtimeConfig: {
    public: {
      // ⚠️ `??` (`||` EMAS): bo'sh satr yaroqli qiymat — brauzer API'ga
      // NISBIY yo'l bilan (/api/...) joriy origin orqali boradi (Caddy).
      // `||` bo'sh satrni yutib, localhost'ga qaytarib yuborardi.
      apiBase: process.env.NUXT_PUBLIC_API_BASE ?? 'http://localhost:6081',
    },
  },
  app: {
    head: {
      title: 'Mini-DSS',
      meta: [{ name: 'viewport', content: 'width=device-width, initial-scale=1' }],
      htmlAttrs: { lang: 'uz' },
    },
  },

  // Xavfsizlik sarlavhalari (audit 17-band).
  //
  // ⚠️ CSP ATAYLAB yumshoq: `connect-src` cheklanmagan, chunki brauzer
  // boshqa origin'dagi API'ga boradi (6080 -> 6081) va uning manzili
  // runtime'da o'zgaradi — qattiq ro'yxat API'ni bloklardi. XSS xavfi
  // baribir past: `v-html` ishlatilmaydi, Vue avto-escape qiladi.
  // `'unsafe-inline'` script uchun kerak — Nuxt hydration inline qiladi.
  routeRules: {
    '/**': {
      headers: {
        'X-Frame-Options': 'DENY',
        'X-Content-Type-Options': 'nosniff',
        'Referrer-Policy': 'strict-origin-when-cross-origin',
        'Permissions-Policy': 'camera=(), microphone=(), geolocation=()',
        'Content-Security-Policy':
          "default-src 'self'; " +
          "img-src 'self' blob: data:; " +
          "style-src 'self' 'unsafe-inline'; " +
          "script-src 'self' 'unsafe-inline'; " +
          "frame-ancestors 'none'; " +
          "base-uri 'self'",
      },
    },
  },
})
