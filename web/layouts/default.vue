<script setup lang="ts">
const route = useRoute()
const token = useAuthToken()
const { theme, init, toggle } = useTheme()

onMounted(init)

const nav = [
  { to: '/', label: 'Boshqaruv paneli', icon: 'grid' },
  { to: '/people', label: 'Odamlar', icon: 'users' },
  { to: '/drafts', label: 'Qoralamalar', icon: 'draft' },
  { to: '/reports', label: 'Hisobot', icon: 'report' },
  { to: '/devices', label: 'Qurilmalar', icon: 'device' },
  { to: '/sync', label: 'Sinxronizatsiya', icon: 'sync' },
]

const title = computed(() => nav.find((n) => n.to === route.path)?.label ?? 'Mini-DSS')

async function logout() {
  // Token'ni server tomonda ham bekor qilamiz — shunchaki localStorage'ni
  // tozalash token'ni muddati o'tguncha yaroqli qoldirardi.
  try {
    await useApi().post('/api/logout')
  } catch {
    // Server javob bermasa ham lokal chiqishni davom ettiramiz.
  }
  token.clear()
  navigateTo('/login')
}
</script>

<template>
  <div class="shell">
    <aside class="sidebar">
      <div class="brand">Mini<span>DSS</span></div>

      <nav class="nav">
        <NuxtLink
          v-for="item in nav"
          :key="item.to"
          :to="item.to"
          class="nav-item"
          :class="{ active: route.path === item.to }"
        >
          <NavIcon :name="item.icon" />
          {{ item.label }}
        </NuxtLink>
      </nav>

      <div class="sidebar-foot">Dahua ASI7xxx terminallari</div>
    </aside>

    <div class="main">
      <header class="topbar">
        <h1>{{ title }}</h1>
        <div class="row" style="align-items: center; gap: 10px">
          <button class="sm" :title="theme === 'dark' ? 'Yorug\' mavzu' : 'To\'q mavzu'" @click="toggle">
            {{ theme === 'dark' ? '☀' : '🌙' }}
          </button>
          <span class="dim">{{ token.user() }}</span>
          <button class="sm" @click="logout">Chiqish</button>
        </div>
      </header>

      <main class="content">
        <slot />
      </main>
    </div>
  </div>
</template>
