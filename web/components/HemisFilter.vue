<script setup lang="ts">
/**
 * HEMIS sync filtri — xodim va talaba uchun alohida.
 *
 * HEMIS `student-list` va `employee-list` qo'llab-quvvatlaydigan barcha
 * parametrlar shu yerda. Ro'yxatlar (fakultet, yo'nalish, guruh,
 * klassifikatorlar) API orqali HEMIS'dan olinadi — qo'lda yozilgan kodlar
 * eskirmasin.
 *
 * Ikki filtr bir-biriga TEGISHLI EMAS: maydonlari ham, klassifikatorlari ham
 * boshqa-boshqa.
 */
import type { HemisFilterValue } from '~/composables/useHemisFilter'

const api = useApi()

type Option = { code: string; name: string; parent?: string }
type Ref = { id: number; name: string; code?: string; type?: string; department?: number; specialty?: number }

type Options = {
  education_form: Option[]; education_type: Option[]; payment_form: Option[]
  student_status: Option[]; level: Option[]; semester: Option[]
  gender: Option[]; citizenship: Option[]; accommodation: Option[]
  province: Option[]; district: Option[]
  staff_position: Option[]; employee_status: Option[]
  employment_form: Option[]; employment_staff: Option[]
  employee_type: Option[]; academic_rank: Option[]; academic_degree: Option[]
  departments: Ref[]; specialties: Ref[]; groups: Ref[]; curricula: Ref[]
}

const model = defineModel<HemisFilterValue>({ required: true })

const opts = ref<Options | null>(null)
const loading = ref(true)
const optionsError = ref('')

const count = ref<number | null>(null)
const countApprox = ref(false)
const counting = ref(false)
const countError = ref('')

const empCount = ref<number | null>(null)
const empCounting = ref(false)
const empError = ref('')

onMounted(async () => {
  try {
    opts.value = await api.get<Options>('/api/hemis/filters')
  } catch (e: any) {
    optionsError.value = e.message
  } finally {
    loading.value = false
  }
  refreshCount()
})

const activeFilter = computed(() => compactFilter(model.value.filter))
const activeEmployeeFilter = computed(() => compactFilter(model.value.employee_filter))

const activeCount = computed(() =>
  (model.value.students ? Object.keys(activeFilter.value).length : 0) +
  (model.value.employees ? Object.keys(activeEmployeeFilter.value).length : 0))

/**
 * Nechta xodim va talaba topilishini oldindan ko'rsatamiz.
 *
 * Har bosishda so'rov ketmasin — yozish tugagach (400 ms) yuboriladi.
 */
let timer: any = null
function refreshCount() {
  clearTimeout(timer)
  timer = setTimeout(() => {
    fetchCount(false)
    fetchEmployeeCount()
  }, 400)
}

/**
 * `exact` — turar joy va to'lov shakli ham hisobga olinadi.
 *
 * ⚠️ Aniq sanash butun ro'yxatni tortadi (~15 s), shuning uchun u faqat
 * tugma bosilganda ishlaydi, yozish davomida emas.
 */
async function fetchCount(exact: boolean) {
  if (!model.value.students) { count.value = null; return }

  counting.value = true
  countError.value = ''
  try {
    const res = await api.post<{ count: number; approximate: boolean }>(
      '/api/hemis/student-count' + (exact ? '?exact=1' : ''), activeFilter.value)
    count.value = res.count
    countApprox.value = res.approximate
  } catch (e: any) {
    countError.value = e.message
    count.value = null
  } finally {
    counting.value = false
  }
}

/**
 * Xodim soni — talabanikidan farqli o'laroq ANIQ: xodim filtrlarining
 * hammasi HEMIS so'rovida bajariladi.
 */
async function fetchEmployeeCount() {
  if (!model.value.employees) { empCount.value = null; return }

  empCounting.value = true
  empError.value = ''
  try {
    const res = await api.post<{ count: number }>(
      '/api/hemis/employee-count', activeEmployeeFilter.value)
    empCount.value = res.count
  } catch (e: any) {
    empError.value = e.message
    empCount.value = null
  } finally {
    empCounting.value = false
  }
}

watch(() => [activeFilter.value, model.value.students], refreshCount, { deep: true })
watch(() => [activeEmployeeFilter.value, model.value.employees], refreshCount, { deep: true })

// --- bog'liq ro'yxatlar -------------------------------------------------

const departments = computed(() => opts.value?.departments || [])

// Lavozimlar — 233 ta, shuning uchun API tomonda nom bo'yicha saralangan.
const staffPositions = computed(() => opts.value?.staff_position || [])

const specialties = computed(() => {
  const dep = Number(model.value.filter.department || 0)
  return (opts.value?.specialties || []).filter((s) => !dep || s.department === dep)
})

const groups = computed(() => {
  const dep = Number(model.value.filter.department || 0)
  const spec = Number(model.value.filter.specialty || 0)
  return (opts.value?.groups || []).filter((g) =>
    (!dep || g.department === dep) && (!spec || g.specialty === spec))
})

const curricula = computed(() => {
  const dep = Number(model.value.filter.department || 0)
  const spec = Number(model.value.filter.specialty || 0)
  return (opts.value?.curricula || []).filter((c) =>
    (!dep || c.department === dep) && (!spec || c.specialty === spec))
})

const districts = computed(() => {
  const prov = String(model.value.filter.province || '')
  return (opts.value?.district || []).filter((d) => !prov || d.parent === prov)
})

// Yuqori daraja o'zgarsa, unga to'g'ri kelmay qolgan tanlov tozalanadi —
// aks holda "0 ta topildi" beruvchi imkonsiz juftlik qoladi.
watch(() => model.value.filter.department, () => {
  const f = model.value.filter
  if (f.specialty && !specialties.value.some((s) => s.id === Number(f.specialty))) f.specialty = ''
  if (f.group && !groups.value.some((g) => g.id === Number(f.group))) f.group = ''
  if (f.curriculum && !curricula.value.some((c) => c.id === Number(f.curriculum))) f.curriculum = ''
})
watch(() => model.value.filter.specialty, () => {
  const f = model.value.filter
  if (f.group && !groups.value.some((g) => g.id === Number(f.group))) f.group = ''
  if (f.curriculum && !curricula.value.some((c) => c.id === Number(f.curriculum))) f.curriculum = ''
})
watch(() => model.value.filter.province, () => {
  const f = model.value.filter
  if (f.district && !districts.value.some((d) => d.code === String(f.district))) f.district = ''
})

// --- sana -> unix -------------------------------------------------------

const updatedFrom = ref<string | null>(null)
const updatedTo = ref<string | null>(null)

/** ⚠️ Sana LOKAL yarim tunda hisoblanadi: `Date.parse('2026-01-01')` UTC oladi
 *  va Toshkentda bir kun oldinga surilib ketadi. */
function unixOf(iso: string | null, endOfDay = false): number {
  const parts = (iso || '').split('-').map(Number)
  if (parts.length !== 3 || parts.some(isNaN)) return 0

  const [y, m, d] = parts as [number, number, number]
  const dt = endOfDay ? new Date(y, m - 1, d, 23, 59, 59) : new Date(y, m - 1, d)
  return Math.floor(dt.getTime() / 1000)
}

watch(updatedFrom, (v) => { model.value.filter.updated_at_from = unixOf(v) })
watch(updatedTo, (v) => { model.value.filter.updated_at_to = unixOf(v, true) })

function reset() {
  updatedFrom.value = null
  updatedTo.value = null
  for (const f of [model.value.filter, model.value.employee_filter]) {
    const rec = f as Record<string, string | number>
    for (const k of Object.keys(rec)) rec[k] = ''
  }
}
</script>

<template>
  <div class="panel">
    <div class="panel-head">
      HEMIS filtri
      <span v-if="activeCount" class="pill info">{{ activeCount }} ta shart</span>

      <div class="spacer" />

      <span v-if="empCounting" class="dim"><span class="spinner" />xodim hisoblanmoqda…</span>
      <span v-else-if="empError" class="dim" style="color: var(--err)">{{ empError }}</span>
      <span v-else-if="empCount !== null" class="dim">
        Xodim: <strong>{{ empCount.toLocaleString() }}</strong> ta
      </span>

      <span v-if="counting" class="dim"><span class="spinner" />talaba hisoblanmoqda…</span>
      <span v-else-if="countError" class="dim" style="color: var(--err)">{{ countError }}</span>
      <span v-else-if="count !== null" class="dim">
        Talaba: <strong v-if="!countApprox">{{ count.toLocaleString() }}</strong>
        <template v-else>{{ count.toLocaleString() }} tagacha</template> ta
      </span>

      <BusyButton
        v-if="countApprox"
        class="sm"
        :busy="counting"
        busy-label="Sanalmoqda…"
        @click="fetchCount(true)"
      >
        Aniq sanash
      </BusyButton>
      <button class="sm" @click="reset">Tozalash</button>
    </div>

    <div class="panel-body">
      <div v-if="optionsError" class="alert err" style="font-size: 11.5px">
        Ro'yxatlar olinmadi: {{ optionsError }}
      </div>
      <div v-if="loading" class="loading-box"><span class="spinner" /> Ro'yxatlar yuklanmoqda…</div>

      <!-- Nimani tortish -->
      <div class="row" style="margin-bottom: 12px">
        <label style="display: flex; align-items: center; gap: 6px; margin: 0">
          <input v-model="model.employees" type="checkbox" style="width: auto" />
          Xodimlar
        </label>
        <label style="display: flex; align-items: center; gap: 6px; margin: 0">
          <input v-model="model.students" type="checkbox" style="width: auto" />
          Talabalar
        </label>
      </div>

      <fieldset :disabled="!model.employees" style="border: 0; padding: 0; margin: 0"
        :style="{ opacity: model.employees ? 1 : 0.5 }">
        <div class="dim" style="font-size: 12px; margin-bottom: 6px">Xodim filtri</div>

        <div class="row">
          <div style="flex: 1; min-width: 150px">
            <label>Ro'yxat turi</label>
            <select v-model="model.employee_filter.type" style="width: 100%">
              <option value="">Barchasi</option>
              <option value="teacher">O'qituvchilar</option>
              <option value="employee">Xodimlar</option>
            </select>
          </div>
          <div style="flex: 2; min-width: 200px">
            <label>Bo'lim / kafedra</label>
            <select v-model="model.employee_filter.department" style="width: 100%">
              <option value="">Barchasi</option>
              <option v-for="d in departments" :key="d.id" :value="d.id">
                {{ d.name }}<template v-if="d.type"> · {{ d.type }}</template>
              </option>
            </select>
          </div>
          <div style="flex: 1; min-width: 150px">
            <label>Xodim holati</label>
            <select v-model="model.employee_filter.employee_status" style="width: 100%">
              <option value="">Barchasi</option>
              <option v-for="o in opts?.employee_status || []" :key="o.code" :value="o.code">{{ o.name }}</option>
            </select>
          </div>
          <div style="flex: 1.4; min-width: 180px">
            <label>Lavozim</label>
            <select v-model="model.employee_filter.staff_position" style="width: 100%">
              <option value="">Barchasi</option>
              <option v-for="o in staffPositions" :key="o.code" :value="o.code">{{ o.name }}</option>
            </select>
          </div>
          <div style="flex: 1.4; min-width: 180px">
            <label>Xodim turi</label>
            <select v-model="model.employee_filter.employee_type" style="width: 100%">
              <option value="">Barchasi</option>
              <option v-for="o in opts?.employee_type || []" :key="o.code" :value="o.code">{{ o.name }}</option>
            </select>
          </div>
          <div style="flex: 1.2; min-width: 160px">
            <label>Mehnat shakli</label>
            <select v-model="model.employee_filter.employment_form" style="width: 100%">
              <option value="">Barchasi</option>
              <option v-for="o in opts?.employment_form || []" :key="o.code" :value="o.code">{{ o.name }}</option>
            </select>
          </div>
          <div style="flex: 1; min-width: 130px">
            <label>Stavka</label>
            <select v-model="model.employee_filter.employment_staff" style="width: 100%">
              <option value="">Barchasi</option>
              <option v-for="o in opts?.employment_staff || []" :key="o.code" :value="o.code">{{ o.name }}</option>
            </select>
          </div>
        </div>

        <details style="margin-top: 12px">
          <summary class="dim" style="cursor: pointer; font-size: 12px">
            Xodim uchun qo'shimcha filtrlar
          </summary>

          <div class="row" style="margin-top: 10px">
            <div style="flex: 1; min-width: 140px">
              <label>Ilmiy unvon</label>
              <select v-model="model.employee_filter.academic_rank" style="width: 100%">
                <option value="">Barchasi</option>
                <option v-for="o in opts?.academic_rank || []" :key="o.code" :value="o.code">{{ o.name }}</option>
              </select>
            </div>
            <div style="flex: 1; min-width: 140px">
              <label>Ilmiy daraja</label>
              <select v-model="model.employee_filter.academic_degree" style="width: 100%">
                <option value="">Barchasi</option>
                <option v-for="o in opts?.academic_degree || []" :key="o.code" :value="o.code">{{ o.name }}</option>
              </select>
            </div>
            <div style="flex: 1; min-width: 110px">
              <label>Jinsi</label>
              <select v-model="model.employee_filter.gender" style="width: 100%">
                <option value="">Barchasi</option>
                <option v-for="o in opts?.gender || []" :key="o.code" :value="o.code">{{ o.name }}</option>
              </select>
            </div>
            <div style="flex: 1.4; min-width: 180px">
              <label>Qidiruv (ism yoki xodim ID)</label>
              <input v-model="model.employee_filter.search" placeholder="masalan: 3311911041" style="width: 100%" />
            </div>
            <div style="flex: 1; min-width: 150px">
              <label>Passport JSHSHIR</label>
              <input v-model="model.employee_filter.passport_pin" style="width: 100%" />
            </div>
            <div style="flex: 1; min-width: 150px">
              <label>Passport seriya-raqami</label>
              <input v-model="model.employee_filter.passport_number" placeholder="AA1234567" style="width: 100%" />
            </div>
          </div>
        </details>

        <p class="dim" style="font-size: 11.5px; margin-bottom: 0">
          Ishdan bo'shagan ("Bo'shagan") va faol bo'lmagan xodim filtrdan
          o'tsa ham bazaga QO'SHILMAYDI — mavjudi faolsizlantiriladi va
          terminaldan olib tashlanadi.
        </p>
      </fieldset>

      <hr style="border: 0; border-top: 1px solid var(--border-soft); margin: 14px 0" />

      <fieldset :disabled="!model.students" style="border: 0; padding: 0; margin: 0"
        :style="{ opacity: model.students ? 1 : 0.5 }">
        <div class="dim" style="font-size: 12px; margin-bottom: 6px">Talaba filtri</div>

        <!-- Asosiy -->
        <div class="row">
          <div style="flex: 1.6; min-width: 190px">
            <label>Fakultet / bo'lim</label>
            <select v-model="model.filter.department" style="width: 100%">
              <option value="">Barchasi</option>
              <option v-for="d in departments" :key="d.id" :value="d.id">
                {{ d.name }}<template v-if="d.type"> · {{ d.type }}</template>
              </option>
            </select>
          </div>
          <div style="flex: 1.6; min-width: 190px">
            <label>Yo'nalish</label>
            <select v-model="model.filter.specialty" style="width: 100%">
              <option value="">Barchasi</option>
              <option v-for="s in specialties" :key="s.id" :value="s.id">{{ s.code }} — {{ s.name }}</option>
            </select>
          </div>
          <div style="flex: 1; min-width: 130px">
            <label>Guruh</label>
            <select v-model="model.filter.group" style="width: 100%">
              <option value="">Barchasi</option>
              <option v-for="g in groups" :key="g.id" :value="g.id">{{ g.name }}</option>
            </select>
          </div>
          <div style="flex: 1; min-width: 120px">
            <label>Kurs</label>
            <select v-model="model.filter.level" style="width: 100%">
              <option value="">Barchasi</option>
              <option v-for="o in opts?.level || []" :key="o.code" :value="o.code">{{ o.name }}</option>
            </select>
          </div>
          <div style="flex: 1; min-width: 140px">
            <label>Ta'lim shakli</label>
            <select v-model="model.filter.education_form" style="width: 100%">
              <option value="">Barchasi</option>
              <option v-for="o in opts?.education_form || []" :key="o.code" :value="o.code">{{ o.name }}</option>
            </select>
          </div>
          <div style="flex: 1; min-width: 130px">
            <label>Ta'lim turi</label>
            <select v-model="model.filter.education_type" style="width: 100%">
              <option value="">Barchasi</option>
              <option v-for="o in opts?.education_type || []" :key="o.code" :value="o.code">{{ o.name }}</option>
            </select>
          </div>
          <div style="flex: 1.2; min-width: 160px">
            <label>Turar joy</label>
            <select v-model="model.filter.accommodation" style="width: 100%">
              <option value="">Barchasi</option>
              <option v-for="o in opts?.accommodation || []" :key="o.code" :value="o.code">{{ o.name }}</option>
            </select>
          </div>
          <div style="flex: 1; min-width: 150px">
            <label>Talaba holati</label>
            <select v-model="model.filter.student_status" style="width: 100%">
              <option value="">O'qimoqda (standart)</option>
              <option value="-1">Barcha holatlar</option>
              <option v-for="o in opts?.student_status || []" :key="o.code" :value="o.code">{{ o.name }}</option>
            </select>
          </div>
        </div>

        <!-- Qo'shimcha -->
        <details style="margin-top: 12px">
          <summary class="dim" style="cursor: pointer; font-size: 12px">Qo'shimcha filtrlar</summary>

          <div class="row" style="margin-top: 10px">
            <div style="flex: 1; min-width: 140px">
              <label>To'lov shakli</label>
              <select v-model="model.filter.payment_form" style="width: 100%">
                <option value="">Barchasi</option>
                <option v-for="o in opts?.payment_form || []" :key="o.code" :value="o.code">{{ o.name }}</option>
              </select>
            </div>
            <div style="flex: 1; min-width: 130px">
              <label>Semestr</label>
              <select v-model="model.filter.semester" style="width: 100%">
                <option value="">Barchasi</option>
                <option v-for="o in opts?.semester || []" :key="o.code" :value="o.code">{{ o.name }}</option>
              </select>
            </div>
            <div style="flex: 1; min-width: 110px">
              <label>Jinsi</label>
              <select v-model="model.filter.gender" style="width: 100%">
                <option value="">Barchasi</option>
                <option v-for="o in opts?.gender || []" :key="o.code" :value="o.code">{{ o.name }}</option>
              </select>
            </div>
            <div style="flex: 1.2; min-width: 160px">
              <label>Fuqarolik</label>
              <select v-model="model.filter.citizenship" style="width: 100%">
                <option value="">Barchasi</option>
                <option v-for="o in opts?.citizenship || []" :key="o.code" :value="o.code">{{ o.name }}</option>
              </select>
            </div>
            <div style="flex: 1.2; min-width: 160px">
              <label>Viloyat</label>
              <select v-model="model.filter.province" style="width: 100%">
                <option value="">Barchasi</option>
                <option v-for="o in opts?.province || []" :key="o.code" :value="o.code">{{ o.name }}</option>
              </select>
            </div>
            <div style="flex: 1.2; min-width: 160px">
              <label>Tuman</label>
              <select v-model="model.filter.district" style="width: 100%">
                <option value="">Barchasi</option>
                <option v-for="o in districts" :key="o.code" :value="o.code">{{ o.name }}</option>
              </select>
            </div>
          </div>

          <div class="row" style="margin-top: 10px">
            <div style="flex: 2; min-width: 220px">
              <label>O'quv reja</label>
              <select v-model="model.filter.curriculum" style="width: 100%">
                <option value="">Barchasi</option>
                <option v-for="c in curricula" :key="c.id" :value="c.id">{{ c.name }}</option>
              </select>
            </div>
            <div style="flex: 1.4; min-width: 180px">
              <label>Qidiruv (ism yoki talaba ID)</label>
              <input v-model="model.filter.search" placeholder="masalan: 331261100035" style="width: 100%" />
            </div>
            <div style="flex: 1; min-width: 140px">
              <label>Tyutor JSHSHIR</label>
              <input v-model="model.filter.tutor_pin" style="width: 100%" />
            </div>
          </div>

          <div class="row" style="margin-top: 10px">
            <div style="flex: 1; min-width: 150px">
              <label>Passport JSHSHIR</label>
              <input v-model="model.filter.passport_pin" style="width: 100%" />
            </div>
            <div style="flex: 1; min-width: 150px">
              <label>Passport seriya-raqami</label>
              <input v-model="model.filter.passport_number" placeholder="AA1234567" style="width: 100%" />
            </div>
            <div style="flex: 1; min-width: 150px">
              <label>Yangilangan (dan)</label>
              <DateInput v-model="updatedFrom" placeholder="kun/oy/yil" />
            </div>
            <div style="flex: 1; min-width: 150px">
              <label>Yangilangan (gacha)</label>
              <DateInput v-model="updatedTo" placeholder="kun/oy/yil" />
            </div>
          </div>

          <p class="dim" style="font-size: 11.5px; margin-bottom: 0">
            ⚠️ Passport bo'yicha qidiruvda JSHSHIR va seriya-raqam BIRGA
            beriladi. Turar joy va to'lov shakli HEMIS so'rovida ishlamaydi —
            ular javob ustida saralanadi: sync baribir butun ro'yxatni ko'rib
            chiqadi, lekin bazaga faqat mos tushganlar yoziladi.
            Har bir maydonga bitta qiymat: "kunduzgi + sirtqi" kerak bo'lsa
            sync ikki marta yuritiladi.
          </p>
        </details>
      </fieldset>
    </div>
  </div>
</template>
