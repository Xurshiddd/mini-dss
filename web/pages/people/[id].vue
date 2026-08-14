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

const day = ref(new Date().toISOString().slice(0, 10))
const person = ref<Person | null>(null)
const days = ref<DayRow[]>([])
const events = ref<Event[]>([])
const loading = ref(false)
const error = ref('')

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
onMounted(load)
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

      <!-- Kunlik davomat -->
      <div class="panel">
        <div class="panel-head row" style="justify-content: space-between; align-items: center">
          <span>Kunlik davomat</span>
          <div class="row" style="gap: 6px; align-items: center">
            <button class="sm" @click="shiftDay(-1)">‹</button>
            <div style="width: 140px"><DateInput v-model="day" /></div>
            <button class="sm" @click="shiftDay(1)">›</button>
          </div>
        </div>

        <div class="panel-body">
          <div v-if="loading" class="dim">Yuklanmoqda…</div>
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
