<script setup lang="ts">
definePageMeta({ layout: false })

const api = useApi()
const token = useAuthToken()
const { init } = useTheme()

onMounted(init)

const username = ref('admin')
const password = ref('')
const error = ref('')
const busy = ref(false)

async function submit() {
  error.value = ''
  busy.value = true
  try {
    const res = await api.post<{ token: string; user: string }>('/api/login', {
      username: username.value,
      password: password.value,
    })
    token.set(res.token, res.user)
    await navigateTo('/')
  } catch (e: any) {
    error.value = e.message
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <div class="login-wrap">
    <form class="login-card" @submit.prevent="submit">
      <div class="login-brand">Mini<span>DSS</span></div>
      <p class="login-sub">Kirish nazorati boshqaruv tizimi</p>

      <div v-if="error" class="alert err">{{ error }}</div>

      <div class="field">
        <label for="u">Foydalanuvchi</label>
        <input id="u" v-model="username" autocomplete="username" required style="width: 100%" />
      </div>

      <div class="field">
        <label for="p">Parol</label>
        <input id="p" v-model="password" type="password" autocomplete="current-password"
               required style="width: 100%" />
      </div>

      <button class="primary" type="submit" :disabled="busy" style="width: 100%; margin-top: 4px">
        {{ busy ? 'Tekshirilmoqda…' : 'Kirish' }}
      </button>
    </form>
  </div>
</template>

<style scoped>
.login-wrap {
  height: 100%;
  display: flex;
  align-items: center;
  justify-content: center;
  /* Fon mavzuga ergashadi — qattiq kodlangan to'q rang light mode'da
     butun sahifani buzadi. */
  background:
    radial-gradient(1100px 520px at 50% -10%, var(--bg-hover) 0%, transparent 60%),
    var(--bg);
}
.login-card {
  width: 340px;
  padding: 28px 26px 26px;
  background: var(--bg-panel);
  border: 1px solid var(--border);
  border-radius: 6px;
  box-shadow: 0 18px 50px rgba(0, 0, 0, 0.45);
}
.login-brand { font-size: 22px; font-weight: 600; letter-spacing: 0.4px; }
.login-brand span { color: var(--accent); }
.login-sub { margin: 4px 0 20px; color: var(--text-dim); font-size: 12px; }
</style>
