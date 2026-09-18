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
// Terminalga yuborish tur filtri. Bo'sh = hamma tur.
const deviceTypes = ref<string[]>([])
const syncOptions = ref(emptyHemisFilter())
const photos = ref<HemisProgress>(null)
const hemisConfigured = ref(false)
const devices = ref<DeviceRow[]>([])
const runs = ref<Run[]>([])
const message = ref('')
const error = ref('')
const busy = useBusy()
let poll: any = null

const loaded = ref(false)

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
  } finally {
    loaded.value = true
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
    await api.post('/api/sync/hemis', {
      employees: syncOptions.value.employees,
      students: syncOptions.value.students,
      filter: compactFilter(syncOptions.value.filter),
      employee_filter: compactFilter(syncOptions.value.employee_filter),
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
    const res = await api.post<{ started: number; busy: number; devices: number }>(
      '/api/sync/devices', { person_types: deviceTypes.value })
    message.value = `${res.started} ta qurilmada sync boshlandi` +
      (res.busy ? `, ${res.busy} tasi band edi` : '') +
      (deviceTypes.value.length ? ` (${typeLabels(deviceTypes.value)})` : '') + '.'
    await load()
  } catch (e: any) {
    error.value = e.message
  }
}

const personTypeLabel: Record<string, string> = {
  employee: 'xodimlar', student: 'talabalar', other: 'boshqalar',
}

const typeLabels = (types: string[]) =>
  types.map((t) => personTypeLabel[t] || t).join(', ')

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

const activeSyncs = computed(() =>
  Number(Boolean(hemis.value?.running)) +
  Number(Boolean(photos.value?.running)) +
  devices.value.filter((d) => d.progress?.running).length)

function progressWidth(done: number, total: number) {
  return `${Math.min(100, Math.max(0, (done / (total || 1)) * 100))}%`
}

function closeFilterOnEscape(event: KeyboardEvent) {
  if (event.key === 'Escape') showFilter.value = false
}

onMounted(() => window.addEventListener('keydown', closeFilterOnEscape))
onUnmounted(() => window.removeEventListener('keydown', closeFilterOnEscape))
</script>

<template>
  <div class="sync-page">
    <div v-if="message || error" class="sync-notices">
      <div v-if="message" class="alert ok">{{ message }}</div>
      <div v-if="error" class="alert err">{{ error }}</div>
    </div>

    <header class="sync-overview">
      <div>
        <div class="sync-eyebrow">MA'LUMOT ALMASHINUVI</div>
        <div class="sync-title">3 bosqichli sinxronizatsiya</div>
      </div>
      <div class="overview-metrics">
        <div class="overview-metric">
          <span>Faol jarayon</span>
          <strong :class="{ active: activeSyncs }">{{ activeSyncs }}</strong>
        </div>
        <div class="overview-metric">
          <span>Qurilmalar</span>
          <strong>{{ devices.length }}</strong>
        </div>
        <div class="overview-metric">
          <span>Kutilmoqda</span>
          <strong :class="{ warn: totalPending }">{{ totalPending.toLocaleString() }}</strong>
        </div>
      </div>
    </header>

    <section class="sync-workspace">
      <div class="stage-grid">
        <article class="stage-card" :class="{ 'is-running': hemis?.running }">
          <header class="stage-head">
            <span class="stage-number">01</span>
            <div class="stage-heading">
              <h2>HEMIS → platforma</h2>
              <span>Shaxsiy ma'lumotlarni olish</span>
            </div>
            <span class="stage-state" :class="hemis?.running ? 'running' : 'ready'">
              {{ hemis?.running ? 'Jarayonda' : 'Tayyor' }}
            </span>
          </header>

          <div class="stage-body">
            <p class="stage-description">
              Xodim va talabalar HEMIS'dan bazaga yuklanadi. Bu bosqich terminallarga
              ma'lumot yozmaydi.
            </p>

            <div v-if="!hemisConfigured" class="compact-alert">
              HEMIS sozlanmagan. <span class="mono">.env</span> faylida manzil va tokenni kiriting.
            </div>

            <template v-if="hemis">
              <div v-if="hemis.running" class="progress-meta">
                <span>{{ hemis.stage === 'employees' ? 'Xodimlar' : 'Talabalar' }}</span>
                <span v-if="hemis.expected">{{ hemis.total.toLocaleString() }} / {{ hemis.expected.toLocaleString() }}</span>
              </div>
              <div v-if="hemis.running && hemis.expected" class="progress stage-progress">
                <div :style="{ width: progressWidth(hemis.total, hemis.expected) }" />
              </div>
              <div class="stage-stats">
                <div><span>Jami</span><strong>{{ hemis.total.toLocaleString() }}</strong></div>
                <div><span>Yangi</span><strong class="ok">{{ hemis.created.toLocaleString() }}</strong></div>
                <div><span>Yangilandi</span><strong>{{ hemis.updated.toLocaleString() }}</strong></div>
                <div><span>Xato</span><strong :class="{ err: hemis.failed }">{{ hemis.failed.toLocaleString() }}</strong></div>
              </div>
              <div v-if="hemis.error" class="compact-alert">{{ hemis.error }}</div>
            </template>
            <div v-else class="stage-empty">Hali ishga tushirilmagan</div>
          </div>

          <footer class="stage-actions">
            <button class="sm" :disabled="!hemisConfigured" @click="showFilter = true">Filtrlar</button>
            <BusyButton
              class="primary sm"
              :busy="busy.is('hemis')"
              :disabled="hemis?.running || !hemisConfigured"
              busy-label="Boshlanmoqda…"
              @click="busy.run('hemis', runHemis)"
            >
              {{ hemis?.running ? 'Jarayonda…' : 'Sinxronlash' }}
            </BusyButton>
          </footer>
        </article>

        <article class="stage-card" :class="{ 'is-running': photos?.running }">
          <header class="stage-head">
            <span class="stage-number">02</span>
            <div class="stage-heading">
              <h2>Rasmlarni tekshirish</h2>
              <span>Yuklash va sifat nazorati</span>
            </div>
            <span class="stage-state" :class="photos?.running ? 'running' : 'ready'">
              {{ photos?.running ? 'Jarayonda' : 'Tayyor' }}
            </span>
          </header>

          <div class="stage-body">
            <p class="stage-description">
              HEMIS rasmlari yuklanadi va terminal talablariga tekshiriladi. Qo'lda
              qo'yilgan rasmlar o'zgartirilmaydi.
            </p>
            <template v-if="photos">
              <div class="progress-meta">
                <span>{{ photos.running ? 'Tekshirilmoqda' : 'Oxirgi natija' }}</span>
                <span>{{ photos.updated.toLocaleString() }} / {{ photos.total.toLocaleString() }}</span>
              </div>
              <div class="progress stage-progress">
                <div :style="{ width: progressWidth(photos.updated, photos.total) }" />
              </div>
              <div class="stage-stats">
                <div><span>Jami</span><strong>{{ photos.total.toLocaleString() }}</strong></div>
                <div><span>Ishlandi</span><strong>{{ photos.updated.toLocaleString() }}</strong></div>
                <div><span>Yaroqli</span><strong class="ok">{{ photos.created.toLocaleString() }}</strong></div>
                <div><span>Rad etildi</span><strong :class="{ warn: photos.skipped }">{{ photos.skipped.toLocaleString() }}</strong></div>
              </div>
              <div v-if="photos.error" class="compact-alert">{{ photos.error }}</div>
            </template>
            <div v-else class="stage-empty">Hali ishga tushirilmagan</div>
          </div>

          <footer class="stage-actions stage-actions-end">
            <BusyButton
              class="primary sm"
              :busy="busy.is('photos')"
              :disabled="photos?.running"
              busy-label="Boshlanmoqda…"
              @click="busy.run('photos', runPhotos)"
            >
              {{ photos?.running ? 'Jarayonda…' : 'Rasmlarni yuklash' }}
            </BusyButton>
          </footer>
        </article>

        <article class="stage-card device-card" :class="{ 'is-running': devices.some((d) => d.progress?.running) }">
          <header class="stage-head">
            <span class="stage-number">03</span>
            <div class="stage-heading">
              <h2>Platforma → terminallar</h2>
              <span>Foydalanuvchilarni yuborish</span>
            </div>
            <span class="stage-state" :class="devices.some((d) => d.progress?.running) ? 'running' : 'ready'">
              {{ devices.some((d) => d.progress?.running) ? 'Jarayonda' : 'Tayyor' }}
            </span>
          </header>

          <div class="stage-body device-stage-body">
            <p class="stage-description">
              Tekshiruvdan o'tganlar faol terminallarga yuboriladi, nofaol va
              eskirgan yozuvlar olib tashlanadi.
            </p>

            <PersonTypeFilter v-model="deviceTypes" class="device-type-filter" />
            <div v-if="!devices.length" class="stage-empty">Qurilma qo'shilmagan</div>
            <div v-else class="device-list">
              <div v-for="row in devices" :key="row.device.id" class="device-row">
                <div class="device-name">
                  <strong>{{ row.device.name }}</strong>
                  <span class="mono">{{ row.device.ip }}</span>
                </div>
                <div class="device-counts">
                  <span>{{ row.stats.synced.toLocaleString() }} yozilgan</span>
                  <span :class="{ warn: row.stats.pending }">{{ row.stats.pending.toLocaleString() }} kutilmoqda</span>
                </div>
                <div class="device-status">
                  <template v-if="row.progress?.running">
                    <div class="progress"><div :style="{ width: progressWidth(row.progress.done, row.progress.total) }" /></div>
                    <span class="mono">{{ row.progress.done }} / {{ row.progress.total }}</span>
                  </template>
                  <span v-else-if="!row.device.is_active" class="pill mute">O'chirilgan</span>
                  <span v-else-if="row.stats.failed" class="pill err">{{ row.stats.failed }} xato</span>
                  <span v-else class="pill ok">Tayyor</span>
                </div>
              </div>
            </div>
          </div>

          <footer class="stage-actions stage-actions-end">
            <BusyButton
              class="primary sm"
              :busy="busy.is('devices')"
              :disabled="devices.some((d) => d.progress?.running)"
              busy-label="Boshlanmoqda…"
              @click="busy.run('devices', runDevices)"
            >
              Hammasini sinxronlash
            </BusyButton>
          </footer>
        </article>
      </div>

      <section class="history-panel">
        <header class="history-head">
          <div>
            <h2>Sinxronizatsiya tarixi</h2>
            <span>Oxirgi {{ runs.length }} ta jarayon</span>
          </div>
          <BusyButton class="sm" :busy="busy.is('load')" @click="busy.run('load', load)">
            Yangilash
          </BusyButton>
        </header>

        <div v-if="!loaded" class="loading-box"><span class="spinner" /> Yuklanmoqda…</div>
        <div v-else-if="!runs.length" class="empty">Hali sinxronizatsiya bo'lmagan.</div>
        <div v-else class="history-table-wrap">
          <table>
            <thead>
              <tr>
                <th>Boshlandi</th><th>Turi</th><th>Qurilma</th><th>Jami</th>
                <th>Yangi</th><th>Yangilandi</th><th>O'tkazildi</th><th>Xato</th><th>Holat</th>
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
                    {{ run.status === 'done' ? 'Tugadi' : run.status === 'failed' ? 'Xato' : 'Jarayonda' }}
                  </span>
                  <div v-if="run.error" class="run-error" :title="run.error">{{ run.error }}</div>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </section>
    </section>

    <Teleport to="body">
      <div v-if="showFilter && hemisConfigured" class="filter-overlay" role="dialog" aria-modal="true" aria-label="HEMIS filtrlari">
        <button class="filter-backdrop" aria-label="Filtrni yopish" @click="showFilter = false" />
        <aside class="filter-drawer">
          <div class="filter-drawer-head">
            <div><strong>HEMIS filtrlari</strong><span>Kerakli xodim va talabalarni tanlang</span></div>
            <button class="filter-close" aria-label="Yopish" @click="showFilter = false">×</button>
          </div>
          <div class="filter-drawer-body"><HemisFilter v-model="syncOptions" /></div>
        </aside>
      </div>
    </Teleport>
  </div>
</template>

<style scoped>
.sync-page {
  height: calc(100vh - var(--header-h) - 48px);
  min-height: 580px;
  display: grid;
  grid-template-rows: auto minmax(0, 1fr);
  gap: 12px;
  overflow: hidden;
}

.sync-notices { position: fixed; top: 64px; right: 20px; z-index: 20; width: min(420px, calc(100vw - 32px)); }
.sync-notices .alert { box-shadow: 0 8px 30px rgba(0, 0, 0, .28); }

.sync-overview {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 18px;
  padding: 12px 16px;
  background: linear-gradient(100deg, var(--bg-panel), var(--bg-panel-2));
  border: 1px solid var(--border);
  border-radius: 6px;
}
.sync-eyebrow { color: var(--accent); font-size: 10px; font-weight: 700; letter-spacing: 1.4px; }
.sync-title { margin-top: 2px; font-size: 16px; font-weight: 600; }
.overview-metrics { display: flex; align-items: center; }
.overview-metric { min-width: 105px; padding: 0 18px; border-left: 1px solid var(--border); }
.overview-metric span { display: block; color: var(--text-dim); font-size: 10px; text-transform: uppercase; letter-spacing: .5px; }
.overview-metric strong { display: block; margin-top: 1px; font-size: 18px; font-variant-numeric: tabular-nums; }
.overview-metric strong.active { color: var(--accent); }
.warn { color: var(--warn) !important; }
.err { color: var(--err) !important; }
.ok { color: var(--ok) !important; }

.sync-workspace { min-height: 0; display: grid; grid-template-rows: minmax(285px, 1.15fr) minmax(190px, .85fr); gap: 12px; }
.stage-grid { min-height: 0; display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 12px; }
.stage-card {
  min-width: 0;
  min-height: 0;
  display: grid;
  grid-template-rows: auto minmax(0, 1fr) auto;
  background: var(--bg-panel);
  border: 1px solid var(--border);
  border-radius: 6px;
  overflow: hidden;
}
.stage-card.is-running { border-color: color-mix(in srgb, var(--accent) 55%, var(--border)); box-shadow: inset 0 2px 0 var(--accent); }
.stage-head { display: flex; align-items: center; gap: 10px; padding: 11px 12px; border-bottom: 1px solid var(--border-soft); }
.stage-number { display: grid; place-items: center; width: 30px; height: 30px; flex: 0 0 30px; border-radius: 4px; background: var(--accent-soft); color: var(--accent); font-size: 11px; font-weight: 700; }
.stage-heading { min-width: 0; flex: 1; }
.stage-heading h2, .history-head h2 { margin: 0; font-size: 13px; font-weight: 600; }
.stage-heading span, .history-head span { display: block; overflow: hidden; color: var(--text-dim); font-size: 10.5px; text-overflow: ellipsis; white-space: nowrap; }
.stage-state { padding: 2px 7px; border-radius: 10px; font-size: 10px; white-space: nowrap; }
.stage-state.ready { background: rgba(34, 192, 125, .12); color: var(--ok); }
.stage-state.running { background: var(--accent-soft); color: var(--accent); }

.stage-body { min-height: 0; padding: 12px; overflow: auto; scrollbar-width: thin; }
.stage-description { min-height: 38px; margin: 0 0 12px; color: var(--text-dim); font-size: 11.5px; line-height: 1.55; }
.stage-empty { display: grid; min-height: 70px; place-items: center; color: var(--text-faint); font-size: 11.5px; }
.compact-alert { margin: 8px 0; padding: 7px 9px; border: 1px solid rgba(242, 85, 90, .28); border-radius: 4px; background: rgba(242, 85, 90, .08); color: var(--err); font-size: 10.5px; }
.progress-meta { display: flex; justify-content: space-between; gap: 8px; margin-bottom: 5px; color: var(--text-dim); font-size: 10.5px; }
.stage-progress { margin-bottom: 10px; }
.stage-stats { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 6px; }
.stage-stats > div { min-width: 0; padding: 7px 8px; border: 1px solid var(--border-soft); border-radius: 4px; background: var(--bg-panel-2); }
.stage-stats span { display: block; overflow: hidden; color: var(--text-dim); font-size: 9px; text-overflow: ellipsis; text-transform: uppercase; white-space: nowrap; }
.stage-stats strong { display: block; overflow: hidden; margin-top: 2px; font-size: 15px; font-variant-numeric: tabular-nums; text-overflow: ellipsis; }
.stage-actions { display: flex; justify-content: space-between; gap: 8px; padding: 9px 12px; border-top: 1px solid var(--border-soft); background: var(--bg-panel-2); }
.stage-actions-end { justify-content: flex-end; }

.device-stage-body { display: flex; flex-direction: column; }
.device-type-filter { margin-bottom: 10px; }
.device-list { min-height: 0; overflow: auto; border: 1px solid var(--border-soft); border-radius: 4px; scrollbar-width: thin; }
.device-row { display: grid; grid-template-columns: minmax(100px, 1fr) auto minmax(78px, .75fr); align-items: center; gap: 8px; padding: 7px 8px; border-bottom: 1px solid var(--border-soft); }
.device-row:last-child { border-bottom: 0; }
.device-row:hover { background: var(--bg-hover); }
.device-name, .device-counts { min-width: 0; }
.device-name strong, .device-name span, .device-counts span { display: block; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.device-name strong { font-size: 11px; }
.device-name span, .device-counts span, .device-status > span { color: var(--text-dim); font-size: 9.5px; }
.device-status { min-width: 0; text-align: right; }
.device-status .progress { margin-bottom: 3px; }

.history-panel { min-height: 0; display: grid; grid-template-rows: auto minmax(0, 1fr); background: var(--bg-panel); border: 1px solid var(--border); border-radius: 6px; overflow: hidden; }
.history-head { display: flex; align-items: center; justify-content: space-between; gap: 12px; padding: 8px 12px; border-bottom: 1px solid var(--border-soft); }
.history-table-wrap { min-height: 0; overflow: auto; scrollbar-width: thin; }
.history-table-wrap thead { position: sticky; top: 0; z-index: 1; }
.history-table-wrap td, .history-table-wrap th { padding-top: 6px; padding-bottom: 6px; }
.run-error { max-width: 180px; margin-top: 2px; overflow: hidden; color: var(--err); font-family: "Cascadia Mono", Consolas, monospace; font-size: 9.5px; text-overflow: ellipsis; white-space: nowrap; }

.filter-overlay { position: fixed; inset: 0; z-index: 100; display: flex; justify-content: flex-end; }
.filter-backdrop { position: absolute; inset: 0; width: 100%; border: 0; border-radius: 0; background: rgba(3, 8, 14, .68); cursor: default; backdrop-filter: blur(2px); }
.filter-drawer { position: relative; width: min(920px, 92vw); height: 100%; display: grid; grid-template-rows: auto minmax(0, 1fr); background: var(--bg); border-left: 1px solid var(--border); box-shadow: -14px 0 40px rgba(0, 0, 0, .35); }
.filter-drawer-head { display: flex; align-items: center; justify-content: space-between; gap: 16px; min-height: 58px; padding: 10px 16px; background: var(--bg-panel); border-bottom: 1px solid var(--border); }
.filter-drawer-head strong, .filter-drawer-head span { display: block; }
.filter-drawer-head span { color: var(--text-dim); font-size: 11px; }
.filter-close { width: 32px; height: 32px; padding: 0; font-size: 22px; line-height: 1; }
.filter-drawer-body { min-height: 0; padding: 14px; overflow: auto; }
.filter-drawer-body :deep(.panel) { margin: 0; }

@media (max-width: 1180px) {
  .sync-page { height: auto; min-height: 0; overflow: visible; }
  .sync-workspace { display: block; }
  .stage-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); }
  .stage-card { min-height: 300px; margin-bottom: 12px; }
  .device-card { grid-column: 1 / -1; min-height: 280px; }
  .history-panel { height: min(440px, 55vh); }
}

@media (max-width: 720px) {
  .sync-overview { align-items: flex-start; flex-direction: column; }
  .overview-metrics { width: 100%; }
  .overview-metric { min-width: 0; flex: 1; padding: 0 10px; }
  .overview-metric:first-child { padding-left: 0; border-left: 0; }
  .stage-grid { display: block; }
  .stage-card, .device-card { min-height: 300px; }
  .stage-stats { grid-template-columns: repeat(2, minmax(0, 1fr)); }
  .device-row { grid-template-columns: 1fr auto; }
  .device-status { grid-column: 1 / -1; text-align: left; }
  .filter-drawer { width: 100%; }
}
</style>
