<script setup lang="ts">
const api = useApi()

type Device = {
  id: number; name: string; ip: string; port: number; username: string
  direction: string; location: string | null
  model: string | null; firmware: string | null; serial: string | null
  is_active: boolean; last_seen_at: string | null; last_error: string | null
}
type Progress = {
  total: number; done: number; users: number; faces: number
  failed: number; removed: number; running: boolean; error?: string
} | null
type Row = { device: Device; stats: { synced: number; pending: number; failed: number; faces: number }; progress: Progress }

const rows = ref<Row[]>([])
const message = ref('')
const error = ref('')
const editing = ref<Partial<Device> & { password?: string } | null>(null)
let poll: any = null

const blank = () => ({ name: '', ip: '', port: 80, username: 'admin', password: '',
                       direction: 'entry', location: '', is_active: true })

async function load() {
  try {
    rows.value = (await api.get<Row[]>('/api/devices')) || []
  } catch (e: any) {
    error.value = e.message
  }
}

// Sync ketayotganda progressni yangilab turamiz.
onMounted(() => {
  load()
  poll = setInterval(() => {
    if (rows.value.some((r) => r.progress?.running)) load()
  }, 2000)
})
onUnmounted(() => clearInterval(poll))

async function save() {
  if (!editing.value) return
  error.value = ''
  try {
    const body = { ...editing.value }
    if (body.id) {
      await api.put(`/api/devices/${body.id}`, body)
      message.value = 'Saqlandi.'
    } else {
      await api.post('/api/devices', body)
      message.value = 'Qurilma qo\'shildi. Ulanishni tekshiring.'
    }
    editing.value = null
    await load()
  } catch (e: any) {
    error.value = e.message
  }
}

async function test(d: Device) {
  error.value = ''; message.value = ''
  try {
    const res = await api.post<{ identity: { model: string }; users: number; time_drift_s: number }>(
      `/api/devices/${d.id}/test`)
    message.value = `${res.identity.model} · ${res.users.toLocaleString()} foydalanuvchi`
    if (Math.abs(res.time_drift_s) > 30) {
      message.value += ` · ⚠ vaqt farqi ${res.time_drift_s}s (NTP o'chiq bo'lishi mumkin)`
    }
    await load()
  } catch (e: any) {
    error.value = e.message
  }
}

// Soatni darhol to'g'rilaydi va NTP'ni yoqadi. Har sinxronizatsiyada vaqt
// baribir yangilanadi — bu tugma shoshilinch holat va tekshirish uchun.
async function fixTime(d: Device) {
  error.value = ''; message.value = ''
  try {
    const res = await api.post<{
      drift_before_s: number; drift_after_s: number
      ntp: { enabled: boolean; address: string }; ntp_error: string
    }>(`/api/devices/${d.id}/time-fix`)
    message.value = `Vaqt to'g'rilandi: ${res.drift_before_s}s → ${res.drift_after_s}s`
    message.value += res.ntp.enabled
      ? ` · NTP yoqilgan (${res.ntp.address})`
      : ` · ⚠ NTP yoqilmadi${res.ntp_error ? ': ' + res.ntp_error : ''}`
    await load()
  } catch (e: any) {
    error.value = e.message
  }
}

async function sync(d: Device, retryFailed = false) {
  error.value = ''; message.value = ''
  try {
    await api.post(`/api/devices/${d.id}/sync`, { retry_failed: retryFailed })
    message.value = `${d.name}: sync boshlandi.`
    await load()
  } catch (e: any) {
    error.value = e.message
  }
}

async function wipe(d: Device) {
  if (!confirm(
    `«${d.name}» qurilmasidagi BARCHA foydalanuvchi o'chiriladi.\n\n` +
    `Bu QAYTARIB BO'LMAYDI — qurilmadagi yuz shablonlarini yuklab olish mumkin emas.\n\n` +
    `Davom etilsinmi?`)) return

  error.value = ''; message.value = ''
  try {
    await api.post(`/api/devices/${d.id}/wipe`, { confirm: 'WIPE' })
    message.value = `${d.name}: tozalandi.`
    await load()
  } catch (e: any) {
    error.value = e.message
  }
}

// Foydalanuvchi o'chirilganda yuz shabloni qurilmada QOLADI — ular alohida
// saqlanadi va ommaviy o'chirish yo'q. Bu tugma o'shalarni tozalaydi.
// Hozir sync bo'lganlarga tegmaydi.
async function cleanFaces(d: Device) {
  if (!confirm(
    `«${d.name}» qurilmasidagi EGASIZ yuz shablonlari o'chiriladi.\n\n` +
    `Hozir yozilgan odamlarning yuziga TEGILMAYDI.\n\n` +
    `Bu bir necha o'n daqiqa olishi mumkin. Boshlansinmi?`)) return

  error.value = ''; message.value = ''
  try {
    await api.post(`/api/devices/${d.id}/clean-faces`)
    message.value = `${d.name}: yetim yuzlarni tozalash boshlandi.`
    await load()
  } catch (e: any) {
    error.value = e.message
  }
}

async function remove(d: Device) {
  if (!confirm(`«${d.name}» ro'yxatdan o'chirilsinmi? (qurilmaning o'ziga tegilmaydi)`)) return
  try {
    await api.del(`/api/devices/${d.id}`)
    await load()
  } catch (e: any) {
    error.value = e.message
  }
}
</script>

<template>
  <div v-if="message" class="alert ok">{{ message }}</div>
  <div v-if="error" class="alert err">{{ error }}</div>

  <div class="panel">
    <div class="panel-head">
      Qurilmalar
      <button class="primary sm" @click="editing = blank()">Qurilma qo'shish</button>
    </div>

    <div v-if="!rows.length" class="empty">
      Hali qurilma qo'shilmagan. IP va parol shu yerda kiritiladi — bazada shifrlangan holda saqlanadi.
    </div>

    <table v-else>
      <thead>
        <tr>
          <th>Nomi</th>
          <th>Manzil</th>
          <th>Yo'nalish</th>
          <th>Model / Firmware</th>
          <th style="width: 190px">Sync</th>
          <th></th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="row in rows" :key="row.device.id">
          <td>
            <div>{{ row.device.name }}</div>
            <div v-if="row.device.location" class="dim" style="font-size: 11px">
              {{ row.device.location }}
            </div>
            <div v-if="row.device.last_error" class="mono" style="font-size: 10.5px; color: var(--err)">
              {{ row.device.last_error.slice(0, 70) }}
            </div>
          </td>
          <td class="mono dim">{{ row.device.ip }}:{{ row.device.port }}</td>
          <td>
            <span class="pill" :class="row.device.direction === 'entry' ? 'ok' : 'warn'">
              {{ row.device.direction === 'entry' ? 'Kirish' : 'Chiqish' }}
            </span>
          </td>
          <td class="dim">
            <div v-if="row.device.model">{{ row.device.model }}</div>
            <div v-if="row.device.firmware" style="font-size: 11px; opacity: 0.7">
              {{ row.device.firmware }}
            </div>
            <span v-if="!row.device.model">— tekshirilmagan</span>
          </td>

          <td>
            <template v-if="row.progress?.running">
              <div class="progress">
                <div :style="{ width: ((row.progress.done / (row.progress.total || 1)) * 100) + '%' }" />
              </div>
              <div class="dim mono" style="font-size: 11px; margin-top: 3px">
                {{ row.progress.done }} / {{ row.progress.total }}
                <span v-if="row.progress.removed" style="color: var(--warn)">
                  · {{ row.progress.removed }} olib tashlandi
                </span>
                <span v-if="row.progress.failed" style="color: var(--err)">
                  · {{ row.progress.failed }} xato
                </span>
              </div>
            </template>
            <template v-else>
              <span class="mono">{{ row.stats.synced.toLocaleString() }}</span>
              <span class="dim"> yozilgan · </span>
              <span class="mono">{{ row.stats.faces.toLocaleString() }}</span>
              <span class="dim"> yuz</span>
              <div v-if="row.stats.failed" class="mono" style="font-size: 11px; color: var(--err)">
                {{ row.stats.failed }} xato
              </div>
              <div v-if="row.progress?.error" class="mono" style="font-size: 10.5px; color: var(--err)">
                {{ row.progress.error.slice(0, 60) }}
              </div>
            </template>
          </td>

          <td>
            <div class="row" style="justify-content: flex-end; gap: 6px">
              <button class="sm" @click="test(row.device)">Tekshirish</button>
              <button class="sm" @click="fixTime(row.device)">Vaqtni to'g'rilash</button>
              <button class="primary sm" :disabled="row.progress?.running" @click="sync(row.device)">
                Sync
              </button>
              <button v-if="row.stats.failed" class="sm" :disabled="row.progress?.running"
                      @click="sync(row.device, true)">
                Xatolarni qayta
              </button>
              <button class="sm" @click="editing = { ...row.device, password: '' }">Tahrir</button>
              <button class="sm" :disabled="row.progress?.running" @click="cleanFaces(row.device)">
                Yetim yuzlar
              </button>
              <button class="danger sm" @click="wipe(row.device)">Tozalash</button>
              <button class="danger sm" @click="remove(row.device)">O'chirish</button>
            </div>
          </td>
        </tr>
      </tbody>
    </table>
  </div>

  <!-- Qo'shish / tahrirlash -->
  <div v-if="editing" class="panel">
    <div class="panel-head">{{ editing.id ? 'Qurilmani tahrirlash' : 'Yangi qurilma' }}</div>
    <div class="panel-body" style="max-width: 620px">
      <div class="field">
        <label>Nomi</label>
        <input v-model="editing.name" placeholder="Bosh bino — kirish 1" style="width: 100%" />
      </div>

      <div class="row">
        <div class="field" style="flex: 2">
          <label>IP manzil</label>
          <input v-model="editing.ip" class="mono" placeholder="192.168.30.222" style="width: 100%" />
        </div>
        <div class="field" style="flex: 1">
          <label>Port</label>
          <input v-model.number="editing.port" type="number" class="mono" style="width: 100%" />
        </div>
      </div>

      <div class="row">
        <div class="field" style="flex: 1">
          <label>Foydalanuvchi</label>
          <input v-model="editing.username" autocomplete="off" style="width: 100%" />
        </div>
        <div class="field" style="flex: 1">
          <label>Parol {{ editing.id ? '— o\'zgartirmasangiz bo\'sh qoldiring' : '' }}</label>
          <input v-model="editing.password" type="password" autocomplete="new-password" style="width: 100%" />
        </div>
      </div>

      <div class="row">
        <div class="field" style="flex: 1">
          <label>Yo'nalish</label>
          <select v-model="editing.direction" style="width: 100%">
            <option value="entry">Kirish</option>
            <option value="exit">Chiqish</option>
          </select>
        </div>
        <div class="field" style="flex: 2">
          <label>Joylashuv — ixtiyoriy</label>
          <input v-model="editing.location" placeholder="1-qavat, sharqiy eshik" style="width: 100%" />
        </div>
      </div>

      <label style="display: flex; align-items: center; gap: 7px; margin-bottom: 14px">
        <input v-model="editing.is_active" type="checkbox" style="min-width: auto" />
        <span style="color: var(--text)">Faol — sync shu qurilmaga yuboriladi</span>
      </label>

      <div class="row">
        <button class="primary" @click="save">{{ editing.id ? 'Saqlash' : 'Qo\'shish' }}</button>
        <button @click="editing = null">Bekor qilish</button>
      </div>

      <p class="dim" style="margin: 14px 0 0; font-size: 11.5px">
        Yo'nalish shu yerda belgilanadi — terminal yozuvlaridagi <span class="mono">Type</span>
        maydoni ishonchli emas, u har doim <span class="mono">Entry</span> qaytaradi.
      </p>
    </div>
  </div>
</template>
