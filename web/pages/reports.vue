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
const anchor = ref(todayISO())
const query = ref('')
const rows = ref<DayRow[]>([])
const loading = ref(false)
const error = ref('')
const message = ref('')
const busy = useBusy()
const showStatistics = ref(false)
const statisticsLoading = ref(false)
const statisticsError = ref('')
const statisticsRows = ref<DayRow[]>([])

// Sahifalash ODAM bo'yicha: API tanlangan odamlarning oraliqdagi hamma
// kunini qaytaradi, shuning uchun haftalik/oylik to'r yarim chiqmaydi.
const perPageOptions = [10, 20, 30, 40, 50]
const perPage = ref(20)
const page = ref(1)
const total = ref(0)

const pageCount = computed(() => Math.max(1, Math.ceil(total.value / perPage.value)))
const firstShown = computed(() => (total.value ? (page.value - 1) * perPage.value + 1 : 0))
const lastShown = computed(() => Math.min(page.value * perPage.value, total.value))

function goTo(p: number) {
  page.value = Math.min(Math.max(1, p), pageCount.value)
}

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

const completedRows = computed(() => rows.value.filter((r) => r.hours !== null))
const averageHours = computed(() => {
  if (!completedRows.value.length) return null
  return completedRows.value.reduce((sum, r) => sum + (r.hours || 0), 0) / completedRows.value.length
})

type TrendRow = { day: string; people: number; hours: number; noExit: number }
const statisticsTrend = computed<TrendRow[]>(() => days.value.map((day) => {
  const dayRows = statisticsRows.value.filter((r) => r.day.slice(0, 10) === day)
  const withHours = dayRows.filter((r) => r.hours !== null)
  return {
    day,
    people: new Set(dayRows.map((r) => r.person_id)).size,
    hours: withHours.length
      ? withHours.reduce((sum, r) => sum + (r.hours || 0), 0) / withHours.length
      : 0,
    noExit: dayRows.filter((r) => r.no_exit).length,
  }
}))

const trendMaxPeople = computed(() =>
  Math.max(1, ...statisticsTrend.value.map((item) => item.people)))

const departmentStatistics = computed(() => {
  const map = new Map<string, { name: string; people: Set<number>; hours: number; completed: number; noExit: number }>()
  for (const row of statisticsRows.value) {
    const name = row.department_name || "Bo'lim ko'rsatilmagan"
    let item = map.get(name)
    if (!item) {
      item = { name, people: new Set(), hours: 0, completed: 0, noExit: 0 }
      map.set(name, item)
    }
    item.people.add(row.person_id)
    if (row.hours !== null) {
      item.hours += row.hours
      item.completed++
    }
    if (row.no_exit) item.noExit++
  }
  return [...map.values()]
    .map((item) => ({
      name: item.name,
      people: item.people.size,
      averageHours: item.completed ? item.hours / item.completed : 0,
      noExit: item.noExit,
    }))
    .sort((a, b) => b.people - a.people)
    .slice(0, 8)
})

const departmentMaxPeople = computed(() =>
  Math.max(1, ...departmentStatistics.value.map((item) => item.people)))

const statisticsPeople = computed(() =>
  new Set(statisticsRows.value.map((row) => row.person_id)).size)
const statisticsNoExit = computed(() =>
  statisticsRows.value.filter((row) => row.no_exit).length)
const statisticsAverageHours = computed(() => {
  const completed = statisticsRows.value.filter((row) => row.hours !== null)
  return completed.length
    ? completed.reduce((sum, row) => sum + (row.hours || 0), 0) / completed.length
    : null
})

function timePosition(value: string | null) {
  if (!value) return 0
  const date = new Date(value)
  const minutes = date.getHours() * 60 + date.getMinutes()
  return Math.min(100, Math.max(0, ((minutes - 7 * 60) / (13 * 60)) * 100))
}

function timelineStyle(row: DayRow) {
  const start = timePosition(row.first_in)
  const end = row.last_out ? timePosition(row.last_out) : 100
  return { left: `${start}%`, width: `${Math.max(1, end - start)}%` }
}

async function openStatistics() {
  showStatistics.value = true
  statisticsLoading.value = true
  statisticsError.value = ''
  try {
    const res = await api.get<{ rows: DayRow[] }>(
      `/api/reports/days?from=${range.value.from}&to=${range.value.to}` +
      `&q=${encodeURIComponent(query.value)}&limit=5000&offset=0`)
    statisticsRows.value = res.rows || []
  } catch (e: any) {
    statisticsError.value = e.message
  } finally {
    statisticsLoading.value = false
  }
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    const res = await api.get<{ rows: DayRow[]; total: number }>(
      `/api/reports/days?from=${range.value.from}&to=${range.value.to}` +
      `&q=${encodeURIComponent(query.value)}` +
      `&limit=${perPage.value}&offset=${(page.value - 1) * perPage.value}`)
    rows.value = res.rows || []
    total.value = res.total || 0

    // Oxirgi sahifadagi odamlar yo'qolsa (qidiruv toraydi yoki oraliq
    // o'zgaradi) bo'sh sahifada qolib ketmaylik.
    if (!rows.value.length && page.value > pageCount.value) {
      goTo(pageCount.value)
    }
  } catch (e: any) {
    error.value = e.message
  } finally {
    loading.value = false
  }
}

/**
 * Filtr o'zgarganda boshidan ko'rsatamiz.
 *
 * Sahifa allaqachon birinchi bo'lsa `load` qo'lda chaqiriladi — aks holda
 * `page` kuzatuvchisi ishlamay qolardi va jadval yangilanmasdi.
 */
function reload() {
  if (page.value !== 1) {
    page.value = 1
    return
  }
  load()
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
  else {
    // 29–31 sanalarda setMonth overflow qilib bir oyni tashlab ketmasin.
    d.setDate(1)
    d.setMonth(d.getMonth() + step)
  }
  anchor.value = iso(d)
}

watch([period, anchor, perPage], reload)
watch(page, load)

// Hodisalar terminaldan jonli tushadi, shuning uchun bugungi kunni
// ko'rayotgan bo'lsak sahifa o'zi yangilanib turadi.
//
// ⚠️ O'tgan kunlar uchun yangilash keraksiz — ular o'zgarmaydi.
let timer: ReturnType<typeof setInterval> | null = null

const showsToday = computed(() => {
  const today = todayISO()
  return range.value.from <= today && today <= range.value.to
})

onMounted(() => {
  load()
  window.addEventListener('keydown', closeStatisticsOnEscape)
  timer = setInterval(() => {
    if (showsToday.value && !loading.value) load()
  }, 15000)
})

onUnmounted(() => {
  if (timer) clearInterval(timer)
  window.removeEventListener('keydown', closeStatisticsOnEscape)
})

function closeStatisticsOnEscape(event: KeyboardEvent) {
  if (event.key === 'Escape') showStatistics.value = false
}
</script>

<template>
  <div class="reports-page">
    <div class="report-toolbar">
      <div class="report-toolbar-main">
        <div class="report-periods">
          <button
            v-for="p in (['kunlik', 'haftalik', 'oylik'] as Period[])"
            :key="p"
            :class="{ active: period === p }"
            @click="period = p"
          >
            {{ p[0].toUpperCase() + p.slice(1) }}
          </button>
        </div>

        <div class="report-date-nav">
          <button class="icon-only" aria-label="Oldingi davr" @click="shift(-1)">‹</button>
          <div class="report-date-input">
            <DateInput v-model="anchor" />
          </div>
          <button class="icon-only" aria-label="Keyingi davr" :disabled="showsToday" @click="shift(1)">›</button>
          <button class="sm" :disabled="showsToday" @click="anchor = todayISO()">Bugun</button>
        </div>

        <div class="report-search">
          <input v-model="query" placeholder="Ism yoki UserID" @keyup.enter="reload" />
        </div>

        <BusyButton class="sm" :busy="loading" @click="reload">Ko'rsatish</BusyButton>
      </div>

      <div class="report-toolbar-actions">
        <button class="sm" :disabled="loading" @click="openStatistics">Statistika</button>
        <BusyButton
          class="sm"
          :busy="busy.is('xlsx')"
          :disabled="loading || !rows.length"
          busy-label="Tayyorlanmoqda…"
          @click="busy.run('xlsx', exportXlsx)"
        >
          XLSX yuklab olish
        </BusyButton>
        <BusyButton
          class="sm"
          :busy="busy.is('import')"
          :disabled="loading"
          busy-label="Terminallardan olinmoqda…"
          @click="busy.run('import', importEvents)"
        >
          Terminaldan yangilash
        </BusyButton>
      </div>

      <div class="report-summary">
        <span class="dim">{{ toUz(range.from) }} — {{ toUz(range.to) }}</span>
        <span class="dim"> · {{ total.toLocaleString() }} odam</span>
        <span v-if="total" class="dim">
          · {{ firstShown }}–{{ lastShown }} ko'rsatilmoqda · {{ rows.length }} kun-yozuv
        </span>
        <span v-if="noExitTotal" class="pill warn" style="margin-left: 8px">
          {{ noExitTotal }} ta chiqishi yozilmagan
        </span>
      </div>
    </div>

    <div v-if="error" class="alert err">{{ error }}</div>
    <p v-if="message" class="dim">{{ message }}</p>

    <!-- Kunlik -->
    <div v-if="period === 'kunlik'" class="panel timeline-panel">
      <div class="panel-head timeline-head">
        <div>
          <strong>Kunlik vaqt chizig'i</strong>
          <span class="dim">{{ toUz(range.from) }}</span>
        </div>
        <div class="timeline-head-stats">
          <span>{{ rows.length }} yozuv</span>
          <span v-if="averageHours !== null">O'rtacha {{ fmtHours(averageHours) }}</span>
          <span v-if="noExitTotal" class="warn-text">{{ noExitTotal }} ta ochiq</span>
        </div>
        <span v-if="loading && rows.length" class="dim" style="font-weight: 400">
          <span class="spinner" />yangilanmoqda
        </span>
      </div>

      <div v-if="loading && !rows.length" class="loading-box">
        <span class="spinner" /> Yuklanmoqda…
      </div>
      <div v-else-if="!rows.length" class="empty">Bu kunda yozuv yo'q.</div>

      <div v-else class="timeline-list">
        <div class="timeline-axis">
          <span>Xodim</span>
          <div><span>07:00</span><span>09:00</span><span>11:00</span><span>13:00</span><span>15:00</span><span>17:00</span><span>20:00</span></div>
          <span>Natija</span>
        </div>
        <div v-for="r in rows" :key="r.person_id" class="timeline-row">
          <div class="timeline-person">
            <NuxtLink :to="`/people/${r.person_id}`">{{ r.full_name }}</NuxtLink>
            <span>{{ r.department_name || r.user_id }}</span>
          </div>
          <div class="timeline-track">
            <div
              v-if="r.first_in"
              class="timeline-shift"
              :class="{ warning: r.no_exit }"
              :style="timelineStyle(r)"
            />
            <span v-if="r.first_in" class="timeline-point entry" :style="{ left: timePosition(r.first_in) + '%' }" />
            <span v-if="r.last_out" class="timeline-point exit" :style="{ left: timePosition(r.last_out) + '%' }" />
          </div>
          <div class="timeline-result">
            <strong class="mono">{{ fmtHours(r.hours) }}</strong>
            <span v-if="r.no_exit" class="warn-text">Chiqishi yo'q</span>
            <span v-else-if="!r.first_in" class="err">Faqat chiqish</span>
            <span v-else>{{ fmtTime(r.first_in) }}–{{ fmtTime(r.last_out) }}</span>
          </div>
        </div>
      </div>
    </div>

    <!-- Haftalik / oylik -->
    <div v-else class="panel">
      <div class="panel-head">
        {{ period === 'haftalik' ? 'Haftalik' : 'Oylik' }} hisobot
        <span class="dim" style="font-weight: 400">
          {{ toUz(range.from) }} — {{ toUz(range.to) }}
        </span>
        <span v-if="loading && people.length" class="dim" style="font-weight: 400">
          <span class="spinner" />yangilanmoqda
        </span>
      </div>

      <div v-if="loading && !people.length" class="loading-box">
        <span class="spinner" /> Yuklanmoqda…
      </div>
      <div v-else-if="!people.length" class="empty">Bu oraliqda yozuv yo'q.</div>

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
    <!-- Sahifalash -->
    <div v-if="total" class="panel">
      <div class="panel-body row" style="align-items: center; gap: 10px">
        <label style="margin: 0" class="dim">Sahifada</label>
        <select v-model.number="perPage" style="width: 80px">
          <option v-for="n in perPageOptions" :key="n" :value="n">{{ n }}</option>
        </select>
        <span class="dim">ta odam</span>

        <div class="spacer" />

        <button class="sm" :disabled="page <= 1 || loading" @click="goTo(1)">« Boshi</button>
        <button class="sm" :disabled="page <= 1 || loading" @click="goTo(page - 1)">‹ Oldingi</button>
        <span class="mono">{{ page }} / {{ pageCount }}</span>
        <button class="sm" :disabled="page >= pageCount || loading" @click="goTo(page + 1)">
          Keyingi ›
        </button>
        <button class="sm" :disabled="page >= pageCount || loading" @click="goTo(pageCount)">
          Oxiri »
        </button>
      </div>
    </div>

    <Teleport to="body">
      <div v-if="showStatistics" class="statistics-overlay" role="dialog" aria-modal="true" aria-label="Davomat statistikasi">
        <button class="statistics-backdrop" aria-label="Statistikani yopish" @click="showStatistics = false" />
        <section class="statistics-dialog">
          <header class="statistics-dialog-head">
            <div>
              <h2>Davomat statistikasi</h2>
              <span>{{ toUz(range.from) }} — {{ toUz(range.to) }}</span>
            </div>
            <button class="statistics-close" aria-label="Yopish" @click="showStatistics = false">×</button>
          </header>

          <div v-if="statisticsLoading" class="loading-box"><span class="spinner" /> Statistika yuklanmoqda…</div>
          <div v-else-if="statisticsError" class="alert err statistics-error">{{ statisticsError }}</div>
          <div v-else class="statistics-body">
            <div class="statistics-kpis">
              <div><span>Kelgan odamlar</span><strong>{{ statisticsPeople.toLocaleString() }}</strong></div>
              <div><span>O'rtacha ish vaqti</span><strong>{{ fmtHours(statisticsAverageHours) }}</strong></div>
              <div><span>Chiqishi yozilmagan</span><strong :class="{ 'warn-text': statisticsNoExit }">{{ statisticsNoExit }}</strong></div>
            </div>

            <div class="statistics-grid">
              <section class="chart-panel">
                <div class="chart-head">
                  <div><h3>Davr dinamikasi</h3><span>Kunlik kelgan odamlar soni</span></div>
                </div>
                <div class="trend-chart">
                  <div v-for="item in statisticsTrend" :key="item.day" class="trend-column">
                    <div class="trend-value">{{ item.people }}</div>
                    <div class="trend-track">
                      <div :style="{ height: (item.people / trendMaxPeople * 100) + '%' }" />
                      <span
                        v-if="item.noExit"
                        class="trend-warning"
                        :title="`${item.noExit} ta chiqishi yozilmagan`"
                      />
                    </div>
                    <div class="trend-label">{{ Number(item.day.slice(8, 10)) }} {{ dowShort(item.day) }}</div>
                  </div>
                </div>
                <div class="chart-note">Sariq nuqta — chiqishi yozilmagan kun</div>
              </section>

              <section class="chart-panel">
                <div class="chart-head">
                  <div><h3>Bo'limlar kesimida</h3><span>Kelgan xodimlar bo'yicha yuqori 8 ta bo'lim</span></div>
                </div>
                <div v-if="!departmentStatistics.length" class="empty compact-empty">Ma'lumot yo'q.</div>
                <div v-else class="department-chart">
                  <div v-for="item in departmentStatistics" :key="item.name" class="department-row">
                    <div class="department-label">
                      <span :title="item.name">{{ item.name }}</span>
                      <strong>{{ item.people }}</strong>
                    </div>
                    <div class="department-track"><div :style="{ width: (item.people / departmentMaxPeople * 100) + '%' }" /></div>
                    <div class="department-meta">
                      <span>O'rtacha {{ fmtHours(item.averageHours) }}</span>
                      <span v-if="item.noExit" class="warn-text">{{ item.noExit }} ta ochiq</span>
                    </div>
                  </div>
                </div>
              </section>
            </div>
          </div>
        </section>
      </div>
    </Teleport>
  </div>
</template>

<style scoped>
.reports-page { display: grid; gap: 12px; }
.report-toolbar { display: grid; grid-template-columns: minmax(0, 1fr) auto; align-items: center; gap: 9px 14px; padding: 10px 12px; border: 1px solid var(--border); border-radius: 6px; background: var(--bg-panel); }
.report-toolbar-main, .report-toolbar-actions, .report-date-nav { display: flex; align-items: center; gap: 6px; }
.report-toolbar-main { min-width: 0; }
.report-toolbar-actions { justify-content: flex-end; }
.report-periods { display: flex; gap: 2px; padding: 2px; border-radius: var(--radius); background: var(--bg-panel-2); }
.report-periods button { padding: 4px 9px; border: 0; background: transparent; color: var(--text-dim); font-size: 11.5px; }
.report-periods button.active { background: var(--bg-panel); color: var(--text); box-shadow: 0 1px 4px rgba(0, 0, 0, .18); }
.report-date-input { width: 138px; }
.report-date-nav .icon-only { min-width: 28px; height: 28px; padding: 0; font-size: 17px; line-height: 1; }
.report-search { min-width: 180px; flex: 1; }
.report-search input { width: 100%; }
.report-summary { grid-column: 1 / -1; padding-top: 8px; border-top: 1px solid var(--border-soft); font-size: 11.5px; }

.timeline-head > div:first-child { display: flex; align-items: baseline; gap: 10px; }
.timeline-head-stats { display: flex; gap: 14px; margin-left: auto; color: var(--text-dim); font-size: 11px; font-weight: 400; }
.warn-text { color: var(--warn) !important; }
.timeline-list { min-width: 740px; }
.timeline-panel { overflow-x: auto; }
.timeline-axis, .timeline-row { display: grid; grid-template-columns: minmax(180px, 1.1fr) minmax(390px, 2.4fr) minmax(115px, .7fr); align-items: center; }
.timeline-axis { position: sticky; top: 0; z-index: 2; min-height: 35px; padding: 0 12px; border-bottom: 1px solid var(--border); background: var(--bg-panel-2); color: var(--text-dim); font-size: 10px; text-transform: uppercase; }
.timeline-axis > div { display: flex; justify-content: space-between; padding: 0 10px; }
.timeline-row { min-height: 48px; padding: 0 12px; border-bottom: 1px solid var(--border-soft); }
.timeline-row:last-child { border-bottom: 0; }
.timeline-row:hover { background: var(--bg-hover); }
.timeline-person { min-width: 0; padding-right: 12px; }
.timeline-person a, .timeline-person span { display: block; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.timeline-person span { color: var(--text-dim); font-size: 10.5px; }
.timeline-track { position: relative; height: 24px; margin: 0 10px; background: repeating-linear-gradient(to right, transparent 0, transparent calc(16.666% - 1px), var(--border-soft) calc(16.666% - 1px), var(--border-soft) 16.666%); }
.timeline-track::after { content: ''; position: absolute; top: 11px; right: 0; left: 0; height: 2px; background: var(--border-soft); }
.timeline-shift { position: absolute; top: 7px; z-index: 1; height: 10px; min-width: 3px; border-radius: 3px; background: var(--accent); }
.timeline-shift.warning { background: linear-gradient(90deg, var(--warn), transparent); }
.timeline-point { position: absolute; top: 6px; z-index: 2; width: 4px; height: 12px; margin-left: -2px; border-radius: 2px; }
.timeline-point.entry { background: var(--ok); }
.timeline-point.exit { background: var(--accent); }
.timeline-result { min-width: 0; text-align: right; }
.timeline-result strong, .timeline-result span { display: block; }
.timeline-result strong { font-size: 12px; }
.timeline-result span { overflow: hidden; color: var(--text-dim); font-size: 10px; text-overflow: ellipsis; white-space: nowrap; }

.statistics-overlay { position: fixed; inset: 0; z-index: 100; display: grid; place-items: center; padding: 22px; }
.statistics-backdrop { position: absolute; inset: 0; width: 100%; border: 0; border-radius: 0; background: rgba(3, 8, 14, .72); cursor: default; backdrop-filter: blur(2px); }
.statistics-dialog { position: relative; width: min(1040px, 96vw); max-height: 90vh; display: grid; grid-template-rows: auto minmax(0, 1fr); overflow: hidden; border: 1px solid var(--border); border-radius: 7px; background: var(--bg); box-shadow: 0 22px 70px rgba(0, 0, 0, .4); }
.statistics-dialog-head { display: flex; align-items: center; justify-content: space-between; gap: 16px; padding: 12px 15px; border-bottom: 1px solid var(--border); background: var(--bg-panel); }
.statistics-dialog-head h2 { margin: 0; font-size: 15px; }
.statistics-dialog-head span { display: block; color: var(--text-dim); font-size: 11px; }
.statistics-close { width: 32px; height: 32px; padding: 0; font-size: 22px; line-height: 1; }
.statistics-body { min-height: 0; padding: 14px; overflow: auto; }
.statistics-error { margin: 14px; }
.statistics-kpis { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 9px; margin-bottom: 10px; }
.statistics-kpis > div { padding: 10px 12px; border: 1px solid var(--border); border-radius: var(--radius); background: var(--bg-panel); }
.statistics-kpis span { display: block; color: var(--text-dim); font-size: 10px; text-transform: uppercase; }
.statistics-kpis strong { display: block; margin-top: 2px; font-size: 21px; font-variant-numeric: tabular-nums; }
.statistics-grid { display: grid; grid-template-columns: minmax(0, 1.4fr) minmax(290px, 1fr); gap: 10px; }
.chart-panel { min-width: 0; padding: 12px; border: 1px solid var(--border); border-radius: var(--radius); background: var(--bg-panel); }
.chart-head h3 { margin: 0; font-size: 12.5px; }
.chart-head span, .chart-note { color: var(--text-dim); font-size: 10.5px; }
.trend-chart { height: 230px; display: flex; align-items: flex-end; gap: 5px; margin-top: 12px; padding-top: 22px; border-bottom: 1px solid var(--border); overflow-x: auto; }
.trend-column { min-width: 28px; height: 100%; flex: 1 0 28px; display: grid; grid-template-rows: 18px minmax(80px, 1fr) 23px; text-align: center; }
.trend-value { color: var(--text-dim); font-size: 9.5px; }
.trend-track { position: relative; display: flex; align-items: flex-end; justify-content: center; }
.trend-track > div { width: min(24px, 65%); min-height: 2px; border-radius: 3px 3px 0 0; background: var(--accent); }
.trend-warning { position: absolute; top: 4px; width: 6px; height: 6px; border-radius: 50%; background: var(--warn); }
.trend-label { padding-top: 5px; color: var(--text-dim); font-size: 9px; white-space: nowrap; }
.chart-note { margin-top: 8px; }
.department-chart { display: grid; gap: 10px; margin-top: 13px; }
.department-label, .department-meta { display: flex; justify-content: space-between; gap: 8px; font-size: 10.5px; }
.department-label span { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.department-label strong { font-weight: 600; }
.department-track { height: 6px; margin: 4px 0; overflow: hidden; border-radius: 3px; background: var(--border-soft); }
.department-track > div { height: 100%; background: var(--accent); }
.department-meta { color: var(--text-dim); font-size: 9.5px; }
.compact-empty { padding: 28px; }

@media (max-width: 980px) {
  .report-toolbar { grid-template-columns: 1fr; }
  .report-toolbar-main { flex-wrap: wrap; }
  .report-toolbar-actions { justify-content: flex-start; }
  .statistics-grid { grid-template-columns: 1fr; }
}
@media (max-width: 620px) {
  .report-periods, .report-search { flex-basis: 100%; }
  .report-periods button { flex: 1; }
  .statistics-overlay { padding: 0; }
  .statistics-dialog { width: 100%; max-height: 100vh; height: 100%; border-radius: 0; }
  .statistics-kpis { grid-template-columns: 1fr; }
}
</style>
