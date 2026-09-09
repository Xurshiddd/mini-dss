<script setup lang="ts">
/**
 * Qoralamalar — terminaldan olingan yuz rasmlari.
 *
 * Nega kerak: eski DSS orqali kiritilgan odamlarning rasmi faqat
 * terminalda qolgan. HEMIS ular uchun yuz bermaydi yoki yaroqsiz rasm
 * qaytaradi — o'shanda terminaldagi nusxa yagona manba.
 *
 * ⚠️ Qoralama biriktirilmaguncha hech kimga ta'sir qilmaydi. Biriktirilgach
 * odamning rasmi bo'lib qoladi va keyingi sync'da terminalga ketadi.
 */
const api = useApi()

type Draft = {
  id: number; user_id: string; full_name: string
  device_id: number | null; device_name: string | null
  file_name: string; status: string; reject_reason: string | null
  person_id: number | null; person_name: string | null
  attached_at: string | null; created_at: string
  match_id: number | null; match_name: string | null
  match_user_id: string | null; match_photo_status: string | null
  match_how: string | null
}
type Stats = {
  total: number; valid: number; rejected: number
  attached: number; unattached: number; matched: number
}
type Pull = {
  running: boolean; stage: string; total: number; expected: number
  created: number; updated: number; skipped: number; failed: number
  error?: string
} | null
type Device = { device: { id: number; name: string; ip: string; is_active: boolean } }
type Person = { id: number; full_name: string; user_id: string; photo_status: string }

const drafts = ref<Draft[]>([])
const stats = ref<Stats | null>(null)
const pull = ref<Pull>(null)
const devices = ref<Device[]>([])
const total = ref(0)
const offset = ref(0)
const limit = 60

const filter = reactive({ q: '', status: '', attached: 'no' })
const pullDevice = ref('')
const pullLimit = ref('')
const error = ref('')
const message = ref('')
const busy = ref(false)
let poll: any = null

const statusPill: Record<string, { label: string; cls: string }> = {
  valid: { label: 'Yaroqli', cls: 'ok' },
  rejected: { label: 'Rad etilgan', cls: 'err' },
  pending: { label: 'Tekshirilmagan', cls: 'warn' },
}

async function load() {
  error.value = ''
  try {
    const params = new URLSearchParams({
      ...filter, limit: String(limit), offset: String(offset.value),
    })
    const [list, st, pr, dev] = await Promise.all([
      api.get<{ data: Draft[]; total: number }>(`/api/drafts?${params}`),
      api.get<Stats>('/api/drafts/stats'),
      api.get<Pull>('/api/sync/drafts'),
      api.get<Device[]>('/api/devices'),
    ])
    drafts.value = list.data || []
    total.value = list.total
    stats.value = st
    pull.value = pr
    devices.value = dev || []
  } catch (e: any) {
    error.value = e.message
  }
}

onMounted(() => {
  load()
  // Yig'ish uzoq davom etadi — ketayotganda holatni yangilab turamiz.
  poll = setInterval(() => { if (pull.value?.running) load() }, 2000)
})
onUnmounted(() => clearInterval(poll))

function search() { offset.value = 0; load() }
function page(delta: number) { offset.value = Math.max(0, offset.value + delta * limit); load() }

async function startPull() {
  error.value = ''; message.value = ''
  try {
    await api.post('/api/sync/drafts', {
      device_id: Number(pullDevice.value) || 0,
      limit: Number(pullLimit.value) || 0,
    })
    message.value = 'Terminaldan yig\'ish boshlandi.'
    await load()
  } catch (e: any) {
    error.value = e.message
  }
}

/** Mos kelganlarni ommaviy biriktirish — avval ko'rib olish (dry run). */
async function autoAttach(dryRun: boolean) {
  error.value = ''; message.value = ''
  busy.value = true
  try {
    const res = await api.post<{ matched: number; attached: number; failed: number }>(
      '/api/drafts/auto-attach', { dry_run: dryRun })

    if (dryRun) {
      if (!res.matched) {
        message.value = 'Bir ma\'noli moslik topilmadi.'
      } else if (confirm(
        `${res.matched} ta qoralama rasmi yo'q yoki yaroqsiz odamlarga mos keldi.\n\n` +
        `Ular biriktiriladi va keyingi sync'da terminalga yuklanadi. Davom etilsinmi?`)) {
        await autoAttach(false)
      }
      return
    }

    message.value = `${res.attached} ta biriktirildi` +
      (res.failed ? `, ${res.failed} tasida xato` : '') + '.'
    await load()
  } catch (e: any) {
    error.value = e.message
  } finally {
    busy.value = false
  }
}

async function attach(draft: Draft, personId: number, personName: string) {
  if (!confirm(`"${draft.full_name || draft.user_id}" rasmi ${personName} ga biriktirilsinmi?\n\n` +
    `Bu odamning hozirgi rasmi o'rniga shu ketadi (eskisi tarixda qoladi).`)) return

  error.value = ''; message.value = ''
  try {
    await api.post(`/api/drafts/${draft.id}/attach`, { person_id: personId })
    message.value = `${personName} ga rasm biriktirildi — keyingi sync'da terminalga boradi.`
    picker.value = null
    await load()
  } catch (e: any) {
    error.value = e.message
  }
}

async function detach(draft: Draft) {
  error.value = ''; message.value = ''
  try {
    await api.post(`/api/drafts/${draft.id}/detach`)
    message.value = 'Biriktirish bekor qilindi.'
    await load()
  } catch (e: any) {
    error.value = e.message
  }
}

async function remove(draft: Draft) {
  if (!confirm(`Qoralama o'chirilsinmi? Biriktirilgan odamlarning rasmi joyida qoladi.`)) return
  error.value = ''; message.value = ''
  try {
    await api.del(`/api/drafts/${draft.id}`)
    await load()
  } catch (e: any) {
    error.value = e.message
  }
}

// ------------------------------------------------------- qo'lda odam tanlash

const picker = ref<number | null>(null)
const pickerQuery = ref('')
const pickerResults = ref<Person[]>([])

async function findPeople() {
  if (pickerQuery.value.trim().length < 2) {
    pickerResults.value = []
    return
  }
  try {
    const res = await api.get<{ data: Person[] }>(
      `/api/people?q=${encodeURIComponent(pickerQuery.value)}&limit=8`)
    pickerResults.value = res.data || []
  } catch (e: any) {
    error.value = e.message
  }
}

function openPicker(draft: Draft) {
  picker.value = picker.value === draft.id ? null : draft.id
  pickerQuery.value = draft.full_name || draft.user_id
  pickerResults.value = []
  if (picker.value) findPeople()
}

const pullPercent = computed(() => {
  const p = pull.value
  if (!p || !p.expected) return 0
  return Math.min(100, Math.round(((p.created + p.skipped + p.failed) / p.expected) * 100))
})
</script>

<template>
  <div v-if="message" class="alert ok">{{ message }}</div>
  <div v-if="error" class="alert err">{{ error }}</div>

  <!-- Qisqacha -->
  <div v-if="stats" class="stats" style="margin-bottom: 14px">
    <div class="stat">
      <div class="stat-label">Jami qoralama</div>
      <div class="stat-value">{{ stats.total.toLocaleString() }}</div>
    </div>
    <div class="stat">
      <div class="stat-label">Yaroqli</div>
      <div class="stat-value ok">{{ stats.valid.toLocaleString() }}</div>
    </div>
    <div class="stat">
      <div class="stat-label">Biriktirilmagan</div>
      <div class="stat-value warn">{{ stats.unattached.toLocaleString() }}</div>
    </div>
    <div class="stat">
      <div class="stat-label">Mos keladi</div>
      <div class="stat-value" :class="{ ok: stats.matched > 0 }">
        {{ stats.matched.toLocaleString() }}
      </div>
    </div>
  </div>

  <!-- Yig'ish -->
  <div class="panel" style="margin-bottom: 14px">
    <div class="panel-head">Terminaldan yig'ish</div>
    <div class="panel-body">
      <div class="row">
        <div style="flex: 1.4; min-width: 180px">
          <label>Terminal</label>
          <select v-model="pullDevice" style="width: 100%">
            <option value="">Avtomatik — yozuvi eng ko'pi</option>
            <option v-for="d in devices" :key="d.device.id" :value="String(d.device.id)">
              {{ d.device.name }} ({{ d.device.ip }})
            </option>
          </select>
        </div>
        <div style="flex: 0 0 130px">
          <label>Cheklov</label>
          <input v-model="pullLimit" placeholder="hammasi" style="width: 100%" />
        </div>
        <button class="primary" :disabled="pull?.running" @click="startPull">
          {{ pull?.running ? 'Ketyapti…' : 'Yig\'ishni boshlash' }}
        </button>
        <button :disabled="busy || !stats?.matched" @click="autoAttach(true)">
          Mos kelganlarni biriktirish{{ stats?.matched ? ` (${stats.matched})` : '' }}
        </button>
      </div>

      <div v-if="pull" style="margin-top: 12px">
        <div class="row" style="align-items: center; gap: 8px">
          <span class="pill" :class="pull.running ? 'info' : (pull.error ? 'err' : 'ok')">
            {{ pull.running ? 'Ketyapti' : (pull.error ? 'Xato' : 'Tugadi') }}
          </span>
          <span class="dim">{{ pull.stage }}</span>
          <span class="mono dim">
            olindi {{ pull.created }} · o'tkazildi {{ pull.skipped }} · xato {{ pull.failed }}
            <template v-if="pull.expected"> / {{ pull.expected }}</template>
          </span>
        </div>
        <div class="progress" style="margin-top: 8px"><div :style="{ width: pullPercent + '%' }" /></div>
        <div v-if="pull.error" class="alert err" style="margin-top: 10px">{{ pull.error }}</div>
      </div>

      <p class="dim" style="font-size: 11.5px; margin: 12px 0 0">
        Terminal tanlanmasa hamma faol terminaldagi yozuvlar sanaladi va eng
        ko'pi olinadi. Yig'ish hech kimning rasmini o'zgartirmaydi — qoralama
        biriktirilgandagina odamning rasmi bo'ladi.
      </p>
    </div>
  </div>

  <!-- Filtr -->
  <div class="panel" style="margin-bottom: 14px">
    <div class="panel-body">
      <div class="row">
        <div style="flex: 2; min-width: 180px">
          <label>Qidiruv</label>
          <input v-model="filter.q" placeholder="Ism yoki UserID" style="width: 100%" @keyup.enter="search" />
        </div>
        <div style="flex: 1; min-width: 130px">
          <label>Rasm holati</label>
          <select v-model="filter.status" style="width: 100%" @change="search">
            <option value="">Barchasi</option>
            <option value="valid">Yaroqli</option>
            <option value="rejected">Rad etilgan</option>
            <option value="pending">Tekshirilmagan</option>
          </select>
        </div>
        <div style="flex: 1; min-width: 130px">
          <label>Biriktirilgan</label>
          <select v-model="filter.attached" style="width: 100%" @change="search">
            <option value="">Barchasi</option>
            <option value="no">Yo'q</option>
            <option value="yes">Ha</option>
          </select>
        </div>
        <button @click="search">Qidirish</button>
      </div>
    </div>
  </div>

  <!-- Ro'yxat -->
  <div v-if="!drafts.length" class="panel">
    <div class="empty">
      Qoralama yo'q. Yuqoridagi "Yig'ishni boshlash" tugmasi terminaldagi
      yuzlarni shu yerga olib keladi.
    </div>
  </div>

  <div v-else class="cards">
    <div v-for="d in drafts" :key="d.id" class="panel card">
      <AuthImage :path="`/api/drafts/${d.id}/photo`" class="shot" />

      <div class="card-body">
        <div class="name">{{ d.full_name || '—' }}</div>
        <div class="mono dim">{{ d.user_id }}</div>

        <div class="row" style="gap: 5px; margin: 8px 0 0">
          <span class="pill" :class="statusPill[d.status]?.cls || 'mute'">
            {{ statusPill[d.status]?.label || d.status }}
          </span>
          <span v-if="d.device_name" class="pill mute">{{ d.device_name }}</span>
        </div>

        <div v-if="d.reject_reason" class="dim" style="font-size: 11px; margin-top: 6px">
          {{ d.reject_reason }}
        </div>

        <!-- Biriktirilgan -->
        <div v-if="d.person_id" style="margin-top: 10px">
          <div class="pill ok">Biriktirilgan</div>
          <div style="font-size: 12px; margin-top: 4px">{{ d.person_name }}</div>
          <button class="sm" style="width: 100%; margin-top: 8px" @click="detach(d)">
            Bekor qilish
          </button>
        </div>

        <!-- Taklif -->
        <template v-else>
          <div v-if="d.match_id" class="match">
            <div class="dim" style="font-size: 11px">
              Taklif ({{ d.match_how === 'id' ? 'UserID mos' : 'ism mos' }}):
            </div>
            <div style="font-size: 12px; font-weight: 600">{{ d.match_name }}</div>
            <div class="mono dim">{{ d.match_user_id }}</div>
            <span class="pill" :class="statusPill[d.match_photo_status || '']?.cls || 'mute'">
              hozirgi rasm: {{ statusPill[d.match_photo_status || '']?.label || d.match_photo_status }}
            </span>
            <button
              class="primary sm"
              style="width: 100%; margin-top: 8px"
              @click="attach(d, d.match_id, d.match_name || '')"
            >
              Biriktirish
            </button>
          </div>
          <div v-else class="dim" style="font-size: 11px; margin-top: 10px">
            Mos odam topilmadi.
          </div>

          <button class="sm" style="width: 100%; margin-top: 6px" @click="openPicker(d)">
            {{ picker === d.id ? 'Yopish' : 'Boshqa odamni tanlash' }}
          </button>

          <div v-if="picker === d.id" class="picker">
            <input
              v-model="pickerQuery"
              placeholder="Ism yoki UserID"
              style="width: 100%"
              @input="findPeople"
            />
            <button
              v-for="p in pickerResults"
              :key="p.id"
              class="sm pick-row"
              @click="attach(d, p.id, p.full_name)"
            >
              <span>{{ p.full_name }}</span>
              <span class="mono dim">{{ p.user_id }}</span>
            </button>
            <div v-if="pickerQuery.length >= 2 && !pickerResults.length" class="dim"
                 style="font-size: 11px; padding: 4px 0">
              Topilmadi.
            </div>
          </div>
        </template>

        <button class="sm" style="width: 100%; margin-top: 6px" @click="remove(d)">
          O'chirish
        </button>
      </div>
    </div>
  </div>

  <div v-if="drafts.length" class="panel" style="margin-top: 14px">
    <div class="panel-body row" style="justify-content: space-between">
      <span class="dim">
        {{ total ? offset + 1 : 0 }}–{{ Math.min(offset + limit, total) }} / {{ total.toLocaleString() }}
      </span>
      <div class="row" style="gap: 6px">
        <button class="sm" :disabled="offset === 0" @click="page(-1)">Oldingi</button>
        <button class="sm" :disabled="offset + limit >= total" @click="page(1)">Keyingi</button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.cards {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(200px, 1fr));
  gap: 12px;
}

.card { display: flex; flex-direction: column; overflow: hidden; }

.shot {
  width: 100%;
  aspect-ratio: 3 / 4;
  object-fit: cover;
  background: var(--bg-panel-2);
  display: block;
}

.card-body { padding: 10px 11px 11px; }
.name { font-weight: 600; font-size: 12.5px; line-height: 1.3; }

.match {
  margin-top: 10px;
  padding: 8px;
  border: 1px solid var(--border-soft);
  border-radius: var(--radius);
  background: var(--bg);
}

.picker { margin-top: 6px; display: grid; gap: 4px; }
.pick-row {
  display: flex;
  justify-content: space-between;
  gap: 6px;
  text-align: left;
  width: 100%;
}
</style>
