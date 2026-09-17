<script setup lang="ts">
/**
 * Xodim profili — to'liq ma'lumot va tanlangan kundagi davomat.
 *
 * Odamlar ro'yxatidagi ko'z belgisidan va hisobotdagi ismdan ochiladi.
 */
const route = useRoute()
const api = useApi()

const personId = Number(route.params.id)

type Person = {
  id: number; user_id: string; legacy_user_id: string | null
  full_name: string; source: string; person_type: string
  department_name: string | null; gender: string | null; status: string | null
  staff_position: string | null; specialty: string | null
  student_group: string | null; level_name: string | null
  photo_path: string | null; photo_status: string; photo_reject_reason: string | null
  is_active: boolean; pending_delete: boolean; synced_on: number
  access_status: string; valid_from: string | null; valid_to: string | null
}
type DayRow = {
  day: string; first_in: string | null; last_out: string | null
  hours: number | null; events: number; no_exit: boolean
}
type Event = {
  happened_at: string; direction: string; device_name: string; location: string | null
}
type AttendancePeriod = 'week' | 'month'

const day = ref(todayISO())
const person = ref<Person | null>(null)
const days = ref<DayRow[]>([])
const events = ref<Event[]>([])
const loading = ref(false)
const error = ref('')
const attendancePeriod = ref<AttendancePeriod>('week')
const attendanceAnchor = ref(todayISO())
const attendanceDays = ref<DayRow[]>([])
const attendanceLoading = ref(false)
const attendanceError = ref('')

const statusPill: Record<string, { label: string; cls: string }> = {
  valid: { label: 'Yaroqli', cls: 'ok' },
  rejected: { label: 'Rad etilgan', cls: 'err' },
  pending: { label: 'Kutilmoqda', cls: 'warn' },
  missing: { label: "Rasm yo'q", cls: 'mute' },
}

const fmtTime = (v: string | null) => {
  if (!v) return '—'
  const d = new Date(v)
  return `${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}`
}

/** Soatni "7:30" ko'rinishida. */
function fmtHours(h: number | null | undefined): string {
  if (h === null || h === undefined) return '—'
  const total = Math.round(h * 60)
  return `${Math.floor(total / 60)}:${String(total % 60).padStart(2, '0')}`
}

const today = computed(() => days.value[0] ?? null)

const iso = (date: Date) => [
  date.getFullYear(),
  String(date.getMonth() + 1).padStart(2, '0'),
  String(date.getDate()).padStart(2, '0'),
].join('-')

function weekStart(value: string) {
  const date = new Date(value + 'T00:00:00')
  const dow = date.getDay() === 0 ? 7 : date.getDay()
  date.setDate(date.getDate() - dow + 1)
  return date
}

const attendanceRange = computed(() => {
  const anchor = new Date(attendanceAnchor.value + 'T00:00:00')
  if (attendancePeriod.value === 'week') {
    const from = weekStart(attendanceAnchor.value)
    const to = new Date(from)
    to.setDate(to.getDate() + 6)
    return { from: iso(from), to: iso(to) }
  }
  return {
    from: iso(new Date(anchor.getFullYear(), anchor.getMonth(), 1)),
    to: iso(new Date(anchor.getFullYear(), anchor.getMonth() + 1, 0)),
  }
})

const attendanceCalendar = computed(() => {
  const byDay = new Map(attendanceDays.value.map((item) => [item.day.slice(0, 10), item]))
  const result: Array<{ date: string; row: DayRow | null }> = []
  const current = new Date(attendanceRange.value.from + 'T00:00:00')
  const end = new Date(attendanceRange.value.to + 'T00:00:00')
  while (current <= end) {
    const date = iso(current)
    result.push({ date, row: byDay.get(date) || null })
    current.setDate(current.getDate() + 1)
  }
  return result
})

const attendanceTotalHours = computed(() =>
  attendanceDays.value.reduce((sum, item) => sum + (item.hours || 0), 0))
const attendanceCompletedDays = computed(() =>
  attendanceDays.value.filter((item) => item.hours !== null).length)
const attendanceAverageHours = computed(() => attendanceCompletedDays.value
  ? attendanceTotalHours.value / attendanceCompletedDays.value
  : null)
const attendanceNoExit = computed(() =>
  attendanceDays.value.filter((item) => item.no_exit).length)

const weekDayShort = (value: string) =>
  ['Yak', 'Du', 'Se', 'Cho', 'Pay', 'Ju', 'Sha'][new Date(value + 'T00:00:00').getDay()]
const isWeekend = (value: string) =>
  [0, 6].includes(new Date(value + 'T00:00:00').getDay())

async function loadAttendance() {
  attendanceLoading.value = true
  attendanceError.value = ''
  try {
    const res = await api.get<{ rows: DayRow[] }>(
      `/api/reports/days?from=${attendanceRange.value.from}&to=${attendanceRange.value.to}` +
      `&person_id=${personId}&limit=1&offset=0`)
    attendanceDays.value = res.rows || []
  } catch (e: any) {
    attendanceError.value = e.message
  } finally {
    attendanceLoading.value = false
  }
}

function shiftAttendance(step: number) {
  const date = new Date(attendanceAnchor.value + 'T00:00:00')
  if (attendancePeriod.value === 'week') date.setDate(date.getDate() + step * 7)
  else {
    date.setDate(1)
    date.setMonth(date.getMonth() + step)
  }
  attendanceAnchor.value = iso(date)
}

function selectAttendanceDay(value: string) {
  day.value = value
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    const res = await api.get<{ person: Person; days: DayRow[]; events: Event[] }>(
      `/api/reports/person/${personId}?from=${day.value}&to=${day.value}`)
    person.value = res.person
    days.value = res.days || []
    events.value = res.events || []
  } catch (e: any) {
    error.value = e.message
  } finally {
    loading.value = false
  }
}

function shiftDay(step: number) {
  const d = new Date(day.value + 'T00:00:00')
  d.setDate(d.getDate() + step)
  day.value = [
    d.getFullYear(),
    String(d.getMonth() + 1).padStart(2, '0'),
    String(d.getDate()).padStart(2, '0'),
  ].join('-')
}

watch(day, load)
watch([attendancePeriod, attendanceAnchor], loadAttendance)
onMounted(() => {
  load()
  loadAttendance()
})
</script>

<template>
  <div style="display: grid; gap: 14px">
    <div class="row" style="align-items: center; gap: 10px">
      <NuxtLink to="/people" class="btn sm">‹ Odamlar</NuxtLink>
      <h2 v-if="person" style="margin: 0; font-size: 16px">{{ person.full_name }}</h2>
    </div>

    <div v-if="error" class="alert err">{{ error }}</div>
    <div v-else-if="!person" class="panel"><div class="empty">Yuklanmoqda…</div></div>

    <template v-else>
      <div class="split">
        <!-- Chap: rasm va holat -->
        <div class="panel">
          <div class="panel-body" style="text-align: center">
            <AuthImage
              v-if="person.photo_path"
              :person-id="person.id"
              class="avatar lg"
            />
            <div v-else class="avatar lg" style="margin: 0 auto" />

            <div style="margin-top: 10px; font-weight: 600">{{ person.full_name }}</div>
            <div class="mono dim" style="font-size: 11.5px">{{ person.user_id }}</div>
            <div v-if="person.legacy_user_id" class="mono dim" style="font-size: 10.5px">
              eski: {{ person.legacy_user_id }}
            </div>

            <div class="row" style="gap: 5px; margin-top: 10px; flex-wrap: wrap; justify-content: center">
              <span class="pill" :class="person.is_active ? 'ok' : 'mute'">
                {{ person.is_active ? 'Faol' : 'Faolsiz' }}
              </span>
              <span v-if="person.pending_delete" class="pill err">O'chirilmoqda</span>
              <span class="pill" :class="statusPill[person.photo_status]?.cls || 'mute'">
                {{ statusPill[person.photo_status]?.label || person.photo_status }}
              </span>
              <span class="pill mute">{{ person.synced_on }} qurilmada</span>
            </div>

            <p v-if="person.photo_reject_reason" class="dim" style="font-size: 11px; margin: 8px 0 0">
              {{ person.photo_reject_reason }}
            </p>
          </div>
        </div>

        <!-- O'ng: ma'lumotlar -->
        <div class="panel">
          <div class="panel-head">Ma'lumotlar</div>
          <table>
            <tbody>
              <tr><td class="dim" style="width: 170px">Turi</td><td>{{ person.person_type }}</td></tr>
              <tr><td class="dim">Jinsi</td><td>{{ person.gender || '—' }}</td></tr>
              <tr><td class="dim">Bo'lim</td><td>{{ person.department_name || '—' }}</td></tr>
              <tr><td class="dim">Lavozim</td><td>{{ person.staff_position || '—' }}</td></tr>
              <tr v-if="person.student_group">
                <td class="dim">Guruh</td>
                <td>
                  {{ person.student_group }}
                  <span v-if="person.level_name" class="dim"> · {{ person.level_name }}</span>
                </td>
              </tr>
              <tr v-if="person.specialty"><td class="dim">Yo'nalish</td><td>{{ person.specialty }}</td></tr>
              <tr><td class="dim">HEMIS holati</td><td>{{ person.status || '—' }}</td></tr>
              <tr><td class="dim">Manba</td><td class="mono">{{ person.source }}</td></tr>
              <tr>
                <td class="dim">Kirish turi</td>
                <td>{{ person.access_status === 'temporary' ? 'Vaqtincha' : 'Doimiy' }}</td>
              </tr>
              <tr>
                <td class="dim">Amal qilish muddati</td>
                <td>
                  {{ toUz(person.valid_from) || '—' }} —
                  {{ toUz(person.valid_to) || 'muddatsiz' }}
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>

      <!-- Hafta / oy kesimidagi davomat -->
      <div class="panel attendance-history">
        <div class="attendance-history-head">
          <div>
            <strong>Davomat tarixi</strong>
            <span>{{ toUz(attendanceRange.from) }} — {{ toUz(attendanceRange.to) }}</span>
          </div>
          <div class="attendance-history-controls">
            <div class="attendance-periods">
              <button :class="{ active: attendancePeriod === 'week' }" @click="attendancePeriod = 'week'">Hafta</button>
              <button :class="{ active: attendancePeriod === 'month' }" @click="attendancePeriod = 'month'">Oy</button>
            </div>
            <button class="sm icon-only" aria-label="Oldingi davr" :disabled="attendanceLoading" @click="shiftAttendance(-1)">‹</button>
            <div class="attendance-anchor"><DateInput v-model="attendanceAnchor" /></div>
            <button class="sm icon-only" aria-label="Keyingi davr" :disabled="attendanceLoading" @click="shiftAttendance(1)">›</button>
          </div>
        </div>

        <div v-if="attendanceError" class="alert err attendance-error">{{ attendanceError }}</div>
        <div v-else-if="attendanceLoading && !attendanceCalendar.length" class="loading-box">
          <span class="spinner" /> Davomat yuklanmoqda…
        </div>
        <div v-else class="attendance-history-body">
          <div class="attendance-summary">
            <div><span>Jami vaqt</span><strong class="mono">{{ fmtHours(attendanceTotalHours) }}</strong></div>
            <div><span>Kelgan kunlar</span><strong>{{ attendanceDays.length }}</strong></div>
            <div><span>Kunlik o'rtacha</span><strong class="mono">{{ fmtHours(attendanceAverageHours) }}</strong></div>
            <div><span>Chiqishi yo'q</span><strong :class="{ 'warn-text': attendanceNoExit }">{{ attendanceNoExit }}</strong></div>
          </div>

          <div class="attendance-calendar" :class="{ monthly: attendancePeriod === 'month' }">
            <button
              v-for="item in attendanceCalendar"
              :key="item.date"
              class="attendance-day"
              :class="{
                weekend: isWeekend(item.date),
                warning: item.row?.no_exit,
                selected: day === item.date,
              }"
              @click="selectAttendanceDay(item.date)"
            >
              <span>{{ weekDayShort(item.date) }} · {{ Number(item.date.slice(8, 10)) }}</span>
              <strong class="mono">{{ item.row ? fmtHours(item.row.hours) : '—' }}</strong>
              <small v-if="item.row">
                {{ fmtTime(item.row.first_in) }} → {{ fmtTime(item.row.last_out) }}
              </small>
              <small v-else>{{ isWeekend(item.date) ? 'Dam olish' : 'Yozuv yo\'q' }}</small>
            </button>
          </div>
        </div>
      </div>

      <!-- Kunlik davomat -->
      <div class="panel">
        <div class="panel-head row" style="justify-content: space-between; align-items: center">
          <span>Kunlik davomat</span>
          <div class="row" style="gap: 6px; align-items: center">
            <button class="sm" :disabled="loading" @click="shiftDay(-1)">‹</button>
            <div style="width: 140px"><DateInput v-model="day" /></div>
            <button class="sm" :disabled="loading" @click="shiftDay(1)">›</button>
            <span v-if="loading" class="spinner" style="margin-left: 2px" />
          </div>
        </div>

        <div class="panel-body">
          <div v-if="loading" class="loading-box"><span class="spinner" /> Yuklanmoqda…</div>
          <div v-else-if="!today" class="dim">Bu kunda yozuv yo'q.</div>

          <div v-else class="row" style="gap: 30px; flex-wrap: wrap; align-items: center">
            <div>
              <div class="dim" style="font-size: 11px">Kirdi</div>
              <div class="mono" style="font-size: 19px">{{ fmtTime(today.first_in) }}</div>
            </div>
            <div>
              <div class="dim" style="font-size: 11px">Chiqdi</div>
              <div class="mono" style="font-size: 19px">{{ fmtTime(today.last_out) }}</div>
            </div>
            <div>
              <div class="dim" style="font-size: 11px">Ish vaqti</div>
              <div class="mono" style="font-size: 19px">{{ fmtHours(today.hours) }}</div>
            </div>
            <div>
              <div class="dim" style="font-size: 11px">O'tishlar</div>
              <div class="mono" style="font-size: 19px">{{ today.events }}</div>
            </div>
            <span v-if="today.no_exit" class="pill warn">
              Chiqishi yozilmagan — boshqa terminaldan o'tgan bo'lishi mumkin
            </span>
          </div>
        </div>

        <div v-if="events.length" style="border-top: 1px solid var(--border-soft)">
          <table>
            <thead>
              <tr><th style="width: 90px">Vaqt</th><th style="width: 110px">Yo'nalish</th><th>Terminal</th></tr>
            </thead>
            <tbody>
              <tr v-for="(e, i) in events" :key="i">
                <td class="mono">{{ fmtTime(e.happened_at) }}</td>
                <td>
                  <span class="pill" :class="e.direction === 'entry' ? 'ok' : 'info'">
                    {{ e.direction === 'entry' ? 'Kirish' : 'Chiqish' }}
                  </span>
                </td>
                <td class="dim">
                  {{ e.device_name }}<span v-if="e.location"> · {{ e.location }}</span>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>
    </template>
  </div>
</template>

<style scoped>
.attendance-history { overflow: hidden; }
.attendance-history-head { display: flex; align-items: center; justify-content: space-between; gap: 14px; padding: 10px 13px; border-bottom: 1px solid var(--border-soft); }
.attendance-history-head > div:first-child strong, .attendance-history-head > div:first-child span { display: block; }
.attendance-history-head > div:first-child span { color: var(--text-dim); font-size: 10.5px; }
.attendance-history-controls { display: flex; align-items: center; gap: 5px; }
.attendance-periods { display: flex; gap: 2px; padding: 2px; border-radius: var(--radius); background: var(--bg-panel-2); }
.attendance-periods button { padding: 3px 9px; border: 0; background: transparent; color: var(--text-dim); font-size: 11px; }
.attendance-periods button.active { background: var(--bg-panel); color: var(--text); box-shadow: 0 1px 4px rgba(0, 0, 0, .18); }
.attendance-history-controls .icon-only { width: 28px; height: 28px; padding: 0; font-size: 17px; line-height: 1; }
.attendance-anchor { width: 138px; }
.attendance-history-body { padding: 12px; }
.attendance-error { margin: 12px; }
.attendance-summary { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 8px; margin-bottom: 10px; }
.attendance-summary > div { padding: 8px 10px; border: 1px solid var(--border-soft); border-radius: var(--radius); background: var(--bg-panel-2); }
.attendance-summary span { display: block; color: var(--text-dim); font-size: 9.5px; text-transform: uppercase; letter-spacing: .3px; }
.attendance-summary strong { display: block; margin-top: 2px; font-size: 17px; }
.warn-text { color: var(--warn); }
.attendance-calendar { display: grid; grid-template-columns: repeat(7, minmax(90px, 1fr)); gap: 6px; overflow-x: auto; }
.attendance-calendar.monthly { grid-template-columns: repeat(7, minmax(95px, 1fr)); }
.attendance-day { min-height: 82px; padding: 7px 8px; border-color: var(--border-soft); background: var(--bg-panel-2); text-align: left; white-space: normal; }
.attendance-day:hover:not(:disabled) { border-color: var(--accent); }
.attendance-day > span, .attendance-day > strong, .attendance-day > small { display: block; }
.attendance-day > span { color: var(--text-dim); font-size: 10px; }
.attendance-day > strong { margin: 7px 0 3px; font-size: 15px; }
.attendance-day > small { overflow: hidden; color: var(--text-faint); font-size: 9.5px; text-overflow: ellipsis; white-space: nowrap; }
.attendance-day.weekend { background: rgba(242, 85, 90, .05); }
.attendance-day.warning { border-color: rgba(232, 177, 58, .6); }
.attendance-day.warning > strong { color: var(--warn); }
.attendance-day.selected { box-shadow: inset 0 0 0 1px var(--accent); }

@media (max-width: 760px) {
  .attendance-history-head { align-items: flex-start; flex-direction: column; }
  .attendance-history-controls { width: 100%; flex-wrap: wrap; }
  .attendance-summary { grid-template-columns: repeat(2, minmax(0, 1fr)); }
  .attendance-calendar { grid-template-columns: repeat(2, minmax(120px, 1fr)); }
  .attendance-calendar.monthly { grid-template-columns: repeat(2, minmax(120px, 1fr)); }
}
</style>
