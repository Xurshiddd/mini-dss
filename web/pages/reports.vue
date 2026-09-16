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
          <!--
            ⚠️ `showsToday` — oraliq bugungi kunni QAMRAB olganmi. Shart
            `anchor` bilan tekshirilmaydi: oylik rejimda anchor 01-avgust,
            bugun 14-avgust bo'lsa ham oraliq bugunni o'z ichiga oladi va
            "Keyingi" kelajakka olib ketardi.
          -->
          <button class="sm" :disabled="showsToday" @click="anchor = todayISO()">Bugun</button>
          <button class="sm" :disabled="showsToday" @click="shift(1)">Keyingi ›</button>
        </div>

        <div style="min-width: 200px; flex: 1">
          <label>Qidiruv</label>
          <input v-model="query" placeholder="Ism yoki UserID" @keyup.enter="reload" />
        </div>

        <BusyButton class="sm" :busy="loading" @click="reload">Ko'rsatish</BusyButton>
        <div class="spacer" />
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

      <div class="panel-body" style="border-top: 1px solid var(--border-soft); padding-top: 10px">
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
    <div v-if="period === 'kunlik'" class="panel">
      <div class="panel-head">
        Kunlik hisobot
        <span class="dim" style="font-weight: 400">{{ toUz(range.from) }}</span>
        <span v-if="loading && rows.length" class="dim" style="font-weight: 400">
          <span class="spinner" />yangilanmoqda
        </span>
      </div>

      <div v-if="loading && !rows.length" class="loading-box">
        <span class="spinner" /> Yuklanmoqda…
      </div>
      <div v-else-if="!rows.length" class="empty">Bu kunda yozuv yo'q.</div>

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
  </div>
</template>
