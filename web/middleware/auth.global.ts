/** Tokensiz hech qayerga kirib bo'lmaydi (login sahifasidan tashqari). */
export default defineNuxtRouteMiddleware((to) => {
  if (!import.meta.client) return

  const token = useAuthToken().get()

  if (!token && to.path !== '/login') {
    return navigateTo('/login')
  }
  if (token && to.path === '/login') {
    return navigateTo('/')
  }
})
