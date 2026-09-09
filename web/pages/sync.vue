<script setup lang="ts">
const api = useApi()

type HemisProgress = {
  running: boolean; stage: string; total: number; expected?: number
  created: number; updated: number; skipped: number; failed: number
  error?: string
} | null

type DeviceRow = {
  device: { id: number; name: string; ip: string; is_active: boolean }
  stats: { synced: number; pending: number; failed: number; faces: number }
  progress: { running: boolean; total: number; done: number; users: number; faces: number; failed: number; removed: number; error?: string } | null
}

type Run = {
  id: number; kind: string; device_name: string | null
  started_at: string; finished_at: string | null
  total: number; ok_count: number; fail_count: number
  created: number; updated: number; skipped: number
  status: string; error: string | null
}

const hemis = ref<HemisProgress>(null)
const showFilter = ref(false)
const syncOptions = ref(emptyHemisFilter())
const photos = ref<HemisProgress>(null)
const hemisConfigured = ref(false)
const devices = ref<DeviceRow[]>([])
const runs = ref<Run[]>([])
const message = ref('')
const error = ref('')
let poll: any = null

async function load() {
  try {
    const [h, ph, d, r] = await Promise.all([
      api.get<{ progress: HemisProgress; configured: boolean }>('/api/sync/hemis'),
      api.get<HemisProgress>('/api/sync/photos'),
      api.get<DeviceRow[]>('/api/devices'),
      api.get<Run[]>('/api/sync/runs?limit=40'),
    ])
    hemis.value = h.progress
    photos.value = ph
    hemisConfigured.value = h.configured
    devices.value = d || []
    runs.value = r || []
  } catch (e: any) {
    error.value = e.message
  }
}

onMounted(() => {
  load()
  poll = setInterval(() => {
    const busy = hemis.value?.running || photos.value?.running ||
      devices.value.some((d) => d.progress?.running)
    if (busy) load()
  }, 2000)
})
onUnmounted(() => clearInterval(poll))

async function runHemis() {
  error.value = ''; message.value = ''
  try {
    // Bo'sh maydonlar yuborilmaydi — HEMIS ularni baribir e'tiborsiz
    // qoldiradi, lekin logda filtr toza ko'rinsin.
    const filter: Record<string, string | number> = {}
    for (const [k, v] of Object.entries(syncOptions.value.filter)) {
      if (v !== '' && v !== 0) filter[k] = v
    }

    await api.post('/api/sync/hemis', {
      employees: syncOptions.value.employees,
      students: syncOptions.value.students,
      filter,
    })
    message.value = 'HEMIS sync boshlandi.'
    await load()
  } catch (e: any) {
    error.value = e.message
  }
}

async function runPhotos() {
  error.value = ''; message.value = ''
  try {
    await api.post('/api/sync/photos')
    message.value = 'Rasm yuklash boshlandi.'
    await load()
  } catch (e: any) {
    error.value = e.message
  }
}

async function runDevices() {
  error.value = ''; message.value = ''
  try {
    const res = await api.post<{ started: number; busy: number; devices: number }>('/api/sync/devices')
    message.value = `${res.started} ta qurilmada sync boshlandi` +
      (res.busy ? `, ${res.busy} tasi band edi` : '') + '.'
    await load()
  } catch (e: any) {
    error.value = e.message
  }
}

const kindLabel: Record<string, string> = {
  hemis_pull: 'HEMIS → platforma',
  photo_fetch: 'Rasmlarni yuklash',
  push_users: 'Platforma → terminal',
  pull_users: 'Terminaldan import',
  draft_pull: 'Terminaldan qoralama yig\'ish',
  wipe: 'Terminal tozalash',
}

const totalPending = computed(() =>
  devices.value.reduce((sum, d) => sum + d.stats.pending, 0))
</script>

<template>
  <div v-if="message" class="alert ok">{{ message }}</div>
  <div v-if="error" class="alert err">{{ error }}</div>

  <HemisFilter v-if="showFilter && hemisConfigured" v-model="syncOptions" />

  <div style="display: grid; grid-template-columns: repeat(3, 1fr); gap: 14px">
    <!-- 1-bosqich: HEMIS -> platforma -->
    <div class="panel">
      <div class="panel-head">
        1 · HEMIS → platforma
        <button class="sm" :disabled="!hemisConfigured" @click="showFilter = !showFilter">
          {{ showFilter ? 'Filtrni yopish' : 'Filtr' }}
        </button>
        <button class="primary sm" :disabled="hemis?.running || !hemisConfigured" @click="runHemis">
          {{ hemis?.running ? 'Ketyapti…' : 'Boshlash' }}
        </button>
      </div>
      <div class="panel-body">
        <p class="dim" style="margin-top: 0">
          Xodim va talabalar HEMIS'dan bazaga tortiladi: ism, ID, jins, tug'ilgan sana,
          bo'lim, lavozim/yo'nalish. Terminalga hech narsa yozilmaydi.
        </p>

        <div v-if="!hemisConfigured" class="alert err" style="font-size: 11.5px">
          HEMIS sozlanmagan — <span class="mono">HEMIS_BASE_URL</span> va
          <span class="mono">HEMIS_TOKEN</span> ni <span class="mono">.env</span> ga qo'shing.
        </div>

        <template v-if="hemis">
          <div v-if="hemis.running" class="dim" style="margin-bottom: 8px">
            Bosqich: {{ hemis.stage === 'employees' ? 'xodimlar' : 'talabalar' }}
            <template v-if="hemis.expected">
              · {{ hemis.total.toLocaleString() }} / {{ hemis.expected.toLocaleString() }}
            </template>
          </div>

          <div v-if="hemis.running && hemis.expected" class="progress" style="margin-bottom: 8px">
            <div :style="{ width: Math.min(100, (hemis.total / hemis.expected) * 100) + '%' }" />
          </div>

          <div class="stats" style="grid-template-columns: repeat(4, 1fr); gap: 8px">
            <div class="stat" style="padding: 8px 10px">
              <div class="stat-label">Jami</div>
              <div class="stat-value" style="font-size: 18px">{{ hemis.total }}</div>
            </div>
            <div class="stat" style="padding: 8px 10px">
              <div class="stat-label">Yangi</div>
              <div class="stat-value ok" style="font-size: 18px">{{ hemis.created }}</div>
            </div>
            <div class="stat" style="padding: 8px 10px">
              <div class="stat-label">Yangilandi</div>
              <div class="stat-value" style="font-size: 18px">{{ hemis.updated }}</div>
            </div>
            <div class="stat" style="padding: 8px 10px">
              <div class="stat-label">Xato</div>
              <div class="stat-value" :class="hemis.failed ? 'err' : ''" style="font-size: 18px">
                {{ hemis.failed }}
              </div>
            </div>
          </div>

          <div v-if="hemis.error" class="alert err" style="margin-top: 10px; font-size: 11.5px">
            {{ hemis.error }}
          </div>
        </template>

        <div v-else class="dim">Hali ishga tushirilmagan.</div>
      </div>
    </div>

    <!-- 2-bosqich: rasmlar -->
    <div class="panel">
      <div class="panel-head">
        2 · Rasmlarni yuklash
        <button class="primary sm" :disabled="photos?.running" @click="runPhotos">
          {{ photos?.running ? 'Ketyapti…' : 'Boshlash' }}
        </button>
      </div>
      <div class="panel-body">
        <p class="dim" style="margin-top: 0">
          HEMIS'dagi rasmlar yuklab olinadi va terminal talablariga tekshiriladi
          (ko'zlar orasi ≥60px, sifat ≥50, bitta yuz). Rasmsiz odam terminalga
          yuborilmaydi.
        </p>

        <template v-if="photos">
          <div class="progress" style="margin-bottom: 8px">
            <div :style="{ width: ((photos.updated / (photos.total || 1)) * 100) + '%' }" />
          </div>

          <div class="stats" style="grid-template-columns: repeat(4, 1fr); gap: 8px">
            <div class="stat" style="padding: 8px 10px">
              <div class="stat-label">Jami</div>
              <div class="stat-value" style="font-size: 18px">{{ photos.total }}</div>
            </div>
            <div class="stat" style="padding: 8px 10px">
              <div class="stat-label">Ishlandi</div>
              <div class="stat-value" style="font-size: 18px">{{ photos.updated }}</div>
            </div>
            <div class="stat" style="padding: 8px 10px">
              <div class="stat-label">Yaroqli</div>
              <div class="stat-value ok" style="font-size: 18px">{{ photos.created }}</div>
            </div>
            <div class="stat" style="padding: 8px 10px">
              <div class="stat-label">Rad etildi</div>
              <div class="stat-value" :class="photos.skipped ? 'warn' : ''" style="font-size: 18px">
                {{ photos.skipped }}
              </div>
            </div>
          </div>
        </template>

        <div v-else class="dim">Hali ishga tushirilmagan.</div>
      </div>
    </div>

    <!-- 3-bosqich: platforma -> terminallar -->
    <div class="panel">
      <div class="panel-head">
        3 · Platforma → terminallar
        <button class="primary sm" :disabled="devices.some((d) => d.progress?.running)" @click="runDevices">
          Hammasini sync qilish
        </button>
      </div>
      <div class="panel-body">
        <p class="dim" style="margin-top: 0">
          Rasmi tekshiruvdan o'tgan odamlar barcha faol terminallarga yoziladi.
          Allaqachon yozilganlar qayta yuborilmaydi — faqat yangilari va rasmi
          o'zgarganlar ketadi.
        </p>
        <p class="dim" style="margin-top: 6px">
          Teskarisi ham ishlaydi: faolsizlantirilgan, muddati tugagan va
          o'chirishga belgilangan odamlar terminaldan olib tashlanadi.
          O'chirilganlar barcha terminaldan tozalangach bazadan yo'qoladi.
        </p>

        <div v-if="!devices.length" class="dim">Qurilma qo'shilmagan.</div>

        <table v-else>
          <thead>
            <tr>
              <th>Qurilma</th>
              <th>Yozilgan</th>
              <th>Kutilmoqda</th>
              <th style="width: 140px">Holat</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="row in devices" :key="row.device.id">
              <td>
                {{ row.device.name }}
                <div class="mono dim" style="font-size: 11px">{{ row.device.ip }}</div>
              </td>
              <td class="mono">{{ row.stats.synced.toLocaleString() }}</td>
              <td class="mono dim">{{ row.stats.pending.toLocaleString() }}</td>
              <td>
                <template v-if="row.progress?.running">
                  <div class="progress">
                    <div :style="{ width: ((row.progress.done / (row.progress.total || 1)) * 100) + '%' }" />
                  </div>
                  <div class="mono dim" style="font-size: 11px; margin-top: 3px">
                    {{ row.progress.done }} / {{ row.progress.total }}
                    <span v-if="row.progress.removed" style="color: var(--warn)">
                      · {{ row.progress.removed }} olib tashlandi
                    </span>
                  </div>
                </template>
                <span v-else-if="!row.device.is_active" class="pill mute">O'chirilgan</span>
                <span v-else-if="row.stats.failed" class="pill err">{{ row.stats.failed }} xato</span>
                <span v-else class="pill ok">Tayyor</span>
              </td>
            </tr>
          </tbody>
        </table>

        <p v-if="totalPending" class="dim" style="margin-bottom: 0; font-size: 11.5px">
          Jami {{ totalPending.toLocaleString() }} ta yozuv kutilmoqda.
        </p>
      </div>
    </div>
  </div>

  <!-- Loglar -->
  <div class="panel">
    <div class="panel-head">
      Sync tarixi
      <button class="sm" @click="load">Yangilash</button>
    </div>

    <div v-if="!runs.length" class="empty">Hali sync bo'lmagan.</div>

    <table v-else>
      <thead>
        <tr>
          <th>Boshlandi</th>
          <th>Turi</th>
          <th>Qurilma</th>
          <th>Jami</th>
          <th>Yangi</th>
          <th>Yangilandi</th>
          <th>O'tkazildi</th>
          <th>Xato</th>
          <th>Holat</th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="run in runs" :key="run.id">
          <td class="mono dim">{{ toUzDateTime(run.started_at) }}</td>
          <td>{{ kindLabel[run.kind] || run.kind }}</td>
          <td class="dim">{{ run.device_name || '—' }}</td>
          <td class="mono">{{ run.total.toLocaleString() }}</td>
          <td class="mono">{{ run.created || '—' }}</td>
          <td class="mono">{{ run.updated || '—' }}</td>
          <td class="mono dim">{{ run.skipped || '—' }}</td>
          <td class="mono" :class="run.fail_count ? 'err' : 'dim'">{{ run.fail_count || '—' }}</td>
          <td>
            <span class="pill" :class="{ ok: run.status === 'done', err: run.status === 'failed', info: run.status === 'running' }">
              {{ run.status === 'done' ? 'Tugadi' : run.status === 'failed' ? 'Xato' : 'Ketyapti' }}
            </span>
            <div v-if="run.error" class="mono" style="font-size: 10.5px; color: var(--err); margin-top: 2px">
              {{ run.error.slice(0, 60) }}
            </div>
          </td>
        </tr>
      </tbody>
    </table>
  </div>
</template>
