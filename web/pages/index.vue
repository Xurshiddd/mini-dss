<script setup lang="ts">
const api = useApi()

type Summary = {
  people: number
  devices: number
  photo_status: Record<string, number>
  sources: Record<string, number>
}
type DeviceRow = {
  device: { id: number; name: string; ip: string; direction: string; model: string | null; last_seen_at: string | null; is_active: boolean }
  stats: { synced: number; pending: number; failed: number; faces: number }
}

const summary = ref<Summary | null>(null)
const devices = ref<DeviceRow[]>([])
const error = ref('')

async function load() {
  try {
    const [s, d] = await Promise.all([
      api.get<Summary>('/api/summary'),
      api.get<DeviceRow[]>('/api/devices'),
    ])
    summary.value = s
    devices.value = d || []
  } catch (e: any) {
    error.value = e.message
  }
}
onMounted(load)

const ready = computed(() => summary.value?.photo_status?.valid ?? 0)
const missing = computed(() => summary.value?.photo_status?.missing ?? 0)
const rejected = computed(() => summary.value?.photo_status?.rejected ?? 0)
</script>

<template>
  <div v-if="error" class="alert err">{{ error }}</div>

  <div class="stats">
    <div class="stat">
      <div class="stat-label">Bazadagi odamlar</div>
      <div class="stat-value">{{ summary?.people?.toLocaleString() ?? '—' }}</div>
    </div>
    <div class="stat">
      <div class="stat-label">Sync'ga tayyor</div>
      <div class="stat-value ok">{{ ready.toLocaleString() }}</div>
    </div>
    <div class="stat">
      <div class="stat-label">Rasmi yo'q</div>
      <div class="stat-value warn">{{ missing.toLocaleString() }}</div>
    </div>
    <div class="stat">
      <div class="stat-label">Rad etilgan rasm</div>
      <div class="stat-value" :class="rejected ? 'err' : ''">{{ rejected.toLocaleString() }}</div>
    </div>
    <div class="stat">
      <div class="stat-label">Qurilmalar</div>
      <div class="stat-value">{{ summary?.devices ?? '—' }}</div>
    </div>
  </div>

  <div class="panel" style="margin-top: 14px">
    <div class="panel-head">
      Qurilmalar holati
      <NuxtLink to="/devices" class="btn sm">Boshqarish</NuxtLink>
    </div>

    <div v-if="!devices.length" class="empty">Hali qurilma qo'shilmagan.</div>

    <table v-else>
      <thead>
        <tr>
          <th>Nomi</th>
          <th>Manzil</th>
          <th>Yo'nalish</th>
          <th>Model</th>
          <th>Yozilgan</th>
          <th>Yuz</th>
          <th>Xato</th>
          <th>Holat</th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="row in devices" :key="row.device.id">
          <td>{{ row.device.name }}</td>
          <td class="mono dim">{{ row.device.ip }}</td>
          <td>
            <span class="pill" :class="row.device.direction === 'entry' ? 'ok' : 'warn'">
              {{ row.device.direction === 'entry' ? 'Kirish' : 'Chiqish' }}
            </span>
          </td>
          <td class="dim">{{ row.device.model || '—' }}</td>
          <td class="mono">{{ row.stats.synced.toLocaleString() }}</td>
          <td class="mono">{{ row.stats.faces.toLocaleString() }}</td>
          <td class="mono" :class="row.stats.failed ? 'err' : 'dim'">{{ row.stats.failed }}</td>
          <td>
            <span v-if="!row.device.is_active" class="pill mute">O'chirilgan</span>
            <span v-else-if="row.device.last_seen_at" class="pill ok">Ulangan</span>
            <span v-else class="pill mute">Tekshirilmagan</span>
          </td>
        </tr>
      </tbody>
    </table>
  </div>
</template>
