<script setup lang="ts">
const api = useApi()

type DayRow = {
  person_id: number; user_id: string; full_name: string
  department_name: string | null; person_type: string
  day: string; first_in: string | null; last_out: string | null
  hours: number | null; events: number; no_exit: boolean
}

type Period = 'kunlik' | 'haftalik' | 'oylik'

const period = ref<Period>('kunlik')
const anchor = ref(new Date().toISOString().slice(0, 10))
const query = ref('')
const rows = ref<DayRow[]>([])
const loading = ref(false)
const error = ref('')
const message = ref('')

/**
 * Hafta DUSHANBADAN boshlanadi.
 *
 * ⚠️ `getDay()` yakshanbani 0 deb qaytaradi, shuning uchun uni 7 ga
 * o'girmasak hafta bir kun oldinga surilib ketadi.
 */
function weekStart(iso: string): Date {
  const d = new Date(iso + 'T00:00:00')
  const dow = d.getDay() === 0 ? 7 : d.getDay()
  d.setDate(d.getDate() - (dow - 1))
  return d
}

const iso = (d: Date) => [
  d.getFullYear(),
  String(d.getMonth() + 1).padStart(2, '0'),
  String(d.getDate()).padStart(2, '0'),
].join('-')

const range = computed<{ from: string; to: string }>(() => {
  const a = new Date(anchor.value + 'T00:00:00')

  if (period.value === 'kunlik') return { from: anchor.value, to: anchor.value }

  if (period.value === 'haftalik') {
    const start = weekStart(anchor.value)
    const end = new Date(start)
    end.setDate(start.getDate() + 6)
    return { from: iso(start), to: iso(end) }
  }

  return {
    from: iso(new Date(a.getFullYear(), a.getMonth(), 1)),
    to: iso(new Date(a.getFullYear(), a.getMonth() + 1, 0)),
  }
})

// Oraliqdagi hamma kun — jadval ustunlari uchun.
const days = computed(() => {
  const out: string[] = []
  const cur = new Date(range.value.from + 'T00:00:00')
  const end = new Date(range.value.to + 'T00:00:00')
  while (cur <= end) {
    out.push(iso(cur))
    cur.setDate(cur.getDate() + 1)
  }
  return out
})

// Shanba va yakshanba — dam olish kunlari.
const isWeekend = (d: string) => [0, 6].includes(new Date(d + 'T00:00:00').getDay())

const dowShort = (d: string) =>
  ['Yak', 'Du', 'Se', 'Cho', 'Pay', 'Ju', 'Sha'][new Date(d + 'T00:00:00').getDay()]

/** Soatni "7:30" ko'rinishida. */
function fmtHours(h: number | null | undefined): string {
  if (h === null || h === undefined) return '—'
  const total = Math.round(h * 60)
  return `${Math.floor(total / 60)}:${String(total % 60).padStart(2, '0')}`
}

const fmtTime = (v: string | null) => {
  if (!v) return '—'
  const d = new Date(v)
  return `${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}`
}

// Odam bo'yicha guruhlangan: haftalik/oylik jadval uchun.
type PersonRow = {
  person_id: number; full_name: string; user_id: string
  department_name: string | null
  byDay: Record<string, DayRow>
  totalHours: number; presentDays: number; noExitDays: number
}

const people = computed<PersonRow[]>(() => {
  const map = new Map<number, PersonRow>()

  for (const r of rows.value) {
    let p = map.get(r.person_id)
    if (!p) {
      p = {
        person_id: r.person_id, full_name: r.full_name, user_id: r.user_id,
        department_name: r.department_name,
        byDay: {}, totalHours: 0, presentDays: 0, noExitDays: 0,
      }
      map.set(r.person_id, p)
    }
    p.byDay[r.day.slice(0, 10)] = r
    p.presentDays++
    if (r.hours) p.totalHours += r.hours
    if (r.no_exit) p.noExitDays++
  }

  return [...map.values()].sort((a, b) => a.full_name.localeCompare(b.full_name))
})

const noExitTotal = computed(() => rows.value.filter((r) => r.no_exit).length)

async function load() {
  loading.value = true
  error.value = ''
  try {
    const res = await api.get<{ rows: DayRow[] }>(
      `/api/reports/days?from=${range.value.from}&to=${range.value.to}` +
      `&q=${encodeURIComponent(query.value)}&limit=5000`)
    rows.value = res.rows || []
  } catch (e: any) {
    error.value = e.message
  } finally {
    loading.value = false
  }
}

// Ekrandagi oraliqni XLSX qilib yuklab beradi.
async function exportXlsx() {
  error.value = ''
  try {
    await api.download(
      `/api/reports/export?from=${range.value.from}&to=${range.value.to}` +
      `&q=${encodeURIComponent(query.value)}`,
      `hisobot-${range.value.from}_${range.value.to}.xlsx`)
  } catch (e: any) {
    error.value = e.message
  }
}

// Terminallardan yangi yozuvlarni tortib oladi.
async function importEvents() {
  loading.value = true
  error.value = ''; message.value = ''
  try {
    const res = await api.post<{ yangi: number; xatolar: Record<string, string> }>(
      '/api/reports/import?pages=25', {})
    const failed = Object.entries(res.xatolar || {})
    message.value = `${res.yangi.toLocaleString()} ta yangi yozuv olindi.`
    if (failed.length) {
      message.value += ` Xato: ${failed.map(([k, v]) => `${k} — ${v}`).join('; ')}`
    }
    await load()
  } catch (e: any) {
    error.value = e.message
  } finally {
    loading.value = false
  }
}

function shift(step: number) {
  const d = new Date(anchor.value + 'T00:00:00')
  if (period.value === 'kunlik') d.setDate(d.getDate() + step)
  else if (period.value === 'haftalik') d.setDate(d.getDate() + step * 7)
  else d.setMonth(d.getMonth() + step)
  anchor.value = iso(d)
}

watch([period, anchor], load)

// Hodisalar terminaldan jonli tushadi, shuning uchun bugungi kunni
// ko'rayotgan bo'lsak sahifa o'zi yangilanib turadi.
//
// ⚠️ O'tgan kunlar uchun yangilash keraksiz — ular o'zgarmaydi.
let timer: ReturnType<typeof setInterval> | null = null

const showsToday = computed(() => {
  const today = new Date().toISOString().slice(0, 10)
  return range.value.from <= today && today <= range.value.to
})

onMounted(() => {
  load()
  timer = setInterval(() => {
    if (showsToday.value && !loading.value) load()
  }, 15000)
})

onUnmounted(() => {
  if (timer) clearInterval(timer)
})
</script>

<template>
  <div style="display: grid; gap: 14px">
    <div class="panel">
      <div class="panel-body row" style="align-items: flex-end; flex-wrap: wrap; gap: 10px">
        <div class="row" style="gap: 4px">
          <button
            v-for="p in (['kunlik', 'haftalik', 'oylik'] as Period[])"
            :key="p"
            class="sm"
            :class="{ primary: period === p }"
            @click="period = p"
          >
            {{ p[0].toUpperCase() + p.slice(1) }}
          </button>
        </div>

        <div style="width: 150px">
          <label>Sana</label>
          <DateInput v-model="anchor" />
        </div>

        <div class="row" style="gap: 4px">
          <button class="sm" @click="shift(-1)">‹ Oldingi</button>
          <button class="sm" @click="shift(1)">Keyingi ›</button>
        </div>

        <div style="min-width: 200px; flex: 1">
          <label>Qidiruv</label>
          <input v-model="query" placeholder="Ism yoki UserID" @keyup.enter="load" />
        </div>

        <button class="sm" @click="load">Ko'rsatish</button>
        <div class="spacer" />
        <button class="sm" :disabled="loading || !rows.length" @click="exportXlsx">
          XLSX yuklab olish
        </button>
        <button class="sm" :disabled="loading" @click="importEvents">
          Terminaldan yangilash
        </button>
      </div>

      <div class="panel-body" style="border-top: 1px solid var(--border-soft); padding-top: 10px">
        <span class="dim">{{ toUz(range.from) }} — {{ toUz(range.to) }}</span>
        <span class="dim"> · {{ people.length }} odam · {{ rows.length }} kun-yozuv</span>
        <span v-if="noExitTotal" class="pill warn" style="margin-left: 8px">
          {{ noExitTotal }} ta chiqishi yozilmagan
        </span>
      </div>
    </div>

    <div v-if="error" class="alert err">{{ error }}</div>
    <p v-if="message" class="dim">{{ message }}</p>

    <!-- Kunlik -->
    <div v-if="period === 'kunlik'" class="panel">
      <div class="panel-head">
        Kunlik hisobot
        <span class="dim" style="font-weight: 400">{{ toUz(range.from) }}</span>
      </div>

      <div v-if="!rows.length" class="empty">
        {{ loading ? 'Yuklanmoqda…' : 'Bu kunda yozuv yo\'q.' }}
      </div>

      <table v-else>
        <thead>
          <tr>
            <th>Ism</th>
            <th>Bo'lim</th>
            <th>Kirdi</th>
            <th>Chiqdi</th>
            <th>Ish vaqti</th>
            <th>Holat</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="r in rows" :key="r.person_id">
            <td>
              <NuxtLink :to="`/people/${r.person_id}`">{{ r.full_name }}</NuxtLink>
              <div class="mono dim" style="font-size: 10.5px">{{ r.user_id }}</div>
            </td>
            <td class="dim">{{ r.department_name || '—' }}</td>
            <td class="mono">{{ fmtTime(r.first_in) }}</td>
            <td class="mono">{{ fmtTime(r.last_out) }}</td>
            <td class="mono">{{ fmtHours(r.hours) }}</td>
            <td>
              <span v-if="r.no_exit" class="pill warn" title="Boshqa chiqishdan o'tgan bo'lishi mumkin">
                Chiqishi yo'q
              </span>
              <span v-else-if="!r.first_in" class="pill mute">Faqat chiqish</span>
              <span v-else class="pill ok">To'liq</span>
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <!-- Haftalik / oylik -->
    <div v-else class="panel">
      <div class="panel-head">
        {{ period === 'haftalik' ? 'Haftalik' : 'Oylik' }} hisobot
        <span class="dim" style="font-weight: 400">
          {{ toUz(range.from) }} — {{ toUz(range.to) }}
        </span>
      </div>

      <div v-if="!people.length" class="empty">
        {{ loading ? 'Yuklanmoqda…' : 'Bu oraliqda yozuv yo\'q.' }}
      </div>

      <div v-else style="overflow-x: auto">
        <table>
          <thead>
            <tr>
              <th style="position: sticky; left: 0; background: var(--bg-panel)">Ism</th>
              <th
                v-for="d in days"
                :key="d"
                :style="{ textAlign: 'center', color: isWeekend(d) ? 'var(--err)' : '' }"
                :title="toUz(d)"
              >
                {{ Number(d.slice(8, 10)) }}
                <div style="font-size: 10px; font-weight: 400">{{ dowShort(d) }}</div>
              </th>
              <th style="text-align: right">Jami</th>
              <th style="text-align: right">Kun</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="p in people" :key="p.person_id">
              <td style="position: sticky; left: 0; background: var(--bg-panel); min-width: 190px">
                <NuxtLink :to="`/people/${p.person_id}`">{{ p.full_name }}</NuxtLink>
                <div class="mono dim" style="font-size: 10.5px">{{ p.user_id }}</div>
              </td>

              <td
                v-for="d in days"
                :key="d"
                class="mono"
                :style="{
                  textAlign: 'center',
                  background: isWeekend(d) ? 'rgba(242, 85, 90, 0.07)' : '',
                  color: p.byDay[d]?.no_exit ? 'var(--warn)' : '',
                }"
                :title="p.byDay[d]
                  ? `${fmtTime(p.byDay[d].first_in)} → ${fmtTime(p.byDay[d].last_out)}`
                  : ''"
              >
                <template v-if="p.byDay[d]">
                  {{ p.byDay[d].hours ? fmtHours(p.byDay[d].hours) : '•' }}
                </template>
                <span v-else class="dim">·</span>
              </td>

              <td class="mono" style="text-align: right">{{ fmtHours(p.totalHours) }}</td>
              <td class="mono dim" style="text-align: right">{{ p.presentDays }}</td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>
  </div>
</template>
