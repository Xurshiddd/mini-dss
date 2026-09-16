<script setup lang="ts">
const api = useApi()

type Person = {
  id: number; user_id: string; legacy_user_id: string | null
  full_name: string; source: string; person_type: string
  department_id: number | null; department_name: string | null
  gender: string | null; status: string | null
  staff_position: string | null; specialty: string | null
  student_group: string | null; level_name: string | null
  photo_path: string | null; photo_status: string; photo_reject_reason: string | null
  photo_source: string
  is_active: boolean; pending_delete: boolean; synced_on: number
  access_status: string; valid_to: string | null
}
type Department = {
  id: number; name: string; parent_id: number | null
  kind: string | null; people: number
}

const people = ref<Person[]>([])
const departments = ref<Department[]>([])
const total = ref(0)
const offset = ref(0)
const limit = 50

const filter = reactive({
  q: '', source: '', photo_status: '', person_type: '', department_id: '', gender: '',
})
const error = ref('')
const message = ref('')
const selected = ref<Person | null>(null)
const checked = ref<Set<number>>(new Set())
const uploading = ref(false)
const loaded = ref(false)
const busy = useBusy()
const editing = ref(false)
const showAdd = ref(false)
const showDeps = ref(false)

const statusPill: Record<string, { label: string; cls: string }> = {
  valid: { label: 'Yaroqli', cls: 'ok' },
  rejected: { label: 'Rad etilgan', cls: 'err' },
  pending: { label: 'Kutilmoqda', cls: 'warn' },
  missing: { label: "Rasm yo'q", cls: 'mute' },
}

// Bo'limlar daraxti — ichma-ich, "—" bilan chuqurlik ko'rsatiladi.
const depTree = computed(() => {
  const byParent = new Map<number | null, Department[]>()
  for (const d of departments.value) {
    const list = byParent.get(d.parent_id) ?? []
    list.push(d)
    byParent.set(d.parent_id, list)
  }
  const out: { dep: Department; depth: number }[] = []
  const walk = (parent: number | null, depth: number) => {
    for (const dep of byParent.get(parent) ?? []) {
      out.push({ dep, depth })
      walk(dep.id, depth + 1)
    }
  }
  walk(null, 0)
  return out
})

async function load() {
  error.value = ''
  try {
    const params = new URLSearchParams({ ...filter, limit: String(limit), offset: String(offset.value) })
    const [res, deps] = await Promise.all([
      api.get<{ data: Person[]; total: number }>(`/api/people?${params}`),
      api.get<Department[]>('/api/departments'),
    ])
    people.value = res.data || []
    total.value = res.total
    departments.value = deps || []
    checked.value = new Set()
  } catch (e: any) {
    error.value = e.message
  } finally {
    loaded.value = true
  }
}
onMounted(load)

function search() { offset.value = 0; load() }
function page(delta: number) { offset.value = Math.max(0, offset.value + delta * limit); load() }

function toggleAll(event: Event) {
  const on = (event.target as HTMLInputElement).checked
  checked.value = on ? new Set(people.value.map((p) => p.id)) : new Set()
}
function toggleOne(id: number) {
  const next = new Set(checked.value)
  next.has(id) ? next.delete(id) : next.add(id)
  checked.value = next
}

// Holat o'zgartirish: tanlanganlarga yoki FILTRGA tushgan hammasiga.
async function setActive(active: boolean, all = false) {
  if (!all && checked.value.size === 0) return
  if (all && !confirm(
    `Filtrga tushgan ${total.value.toLocaleString()} ta odamning hammasi ` +
    `${active ? 'faollashtiriladi' : 'faolsizlantiriladi'}. Davom etilsinmi?`)) return

  error.value = ''; message.value = ''
  try {
    const res = await api.post<{ affected: number }>('/api/people/bulk-status', {
      ids: all ? [] : [...checked.value],
      is_active: active,
      all,
      filter: { ...filter, department_id: Number(filter.department_id) || 0 },
    })
    message.value = `${res.affected.toLocaleString()} ta yozuv ${active ? 'faollashtirildi' : 'faolsizlantirildi'}.`
    await load()
  } catch (e: any) {
    error.value = e.message
  }
}

/**
 * Tanlanganlarni terminalga yuborish.
 *
 * Rasmi tekshiruvdan o'tmagan odam yuborilmaydi — buni oldindan aytamiz,
 * aniq sonini esa server qaytaradi (tanlov sahifadan tashqarida ham
 * bo'lishi mumkin).
 */
const checkedSyncable = computed(() =>
  people.value.filter((p) =>
    checked.value.has(p.id) && p.photo_status === 'valid' &&
    p.is_active && !p.pending_delete).length)

async function pushChecked() {
  if (checked.value.size === 0) return

  const skipped = checked.value.size - checkedSyncable.value
  if (!confirm(
    `${checked.value.size} ta tanlanganlardan ${checkedSyncable.value} tasi ` +
    `barcha faol terminalga yoziladi.` +
    (skipped ? `\n\n${skipped} tasi yuborilmaydi: rasmi yaroqli emas, ` +
               `o'zi nofaol yoki muddati o'tgan.` : '') +
    `\n\nTerminalda allaqachon bo'lganlarning yuzi qaytadan yoziladi.` +
    `\n\nDavom etilsinmi?`)) return

  error.value = ''; message.value = ''
  try {
    const res = await api.post<{
      started: number; busy: number; devices: number; people: number; skipped: number
    }>('/api/sync/selected', { ids: [...checked.value] })

    message.value = `${res.people.toLocaleString()} ta odam ${res.started} ta terminalga yuborilmoqda`
    if (res.busy) message.value += ` · ${res.busy} ta terminal band edi`
    if (res.skipped) message.value += ` · ${res.skipped} tasi yaroqsiz (yuborilmadi)`
    message.value += '. Borishini "Sync" sahifasida kuzating.'
  } catch (e: any) {
    error.value = e.message
  }
}

// O'chirish: darhol emas: avval terminallardan olib tashlanadi, keyin bazadan.
async function removeChecked() {
  if (checked.value.size === 0) return

  const onDevices = people.value
    .filter((p) => checked.value.has(p.id) && p.synced_on > 0).length

  if (!confirm(
    `${checked.value.size} ta odam o'chiriladi.` +
    (onDevices ? `\n\nShundan ${onDevices} tasi terminalda bor — ular keyingi ` +
                 `sync'da terminaldan olib tashlanadi, shundan keyin bazadan o'chadi.` : '') +
    `\n\nDavom etilsinmi?`)) return

  error.value = ''; message.value = ''
  try {
    const res = await api.post<{ affected: number; purged: number; pending: number }>(
      '/api/people/bulk-delete', { ids: [...checked.value] })
    message.value = `${res.affected.toLocaleString()} ta belgilandi · ${res.purged.toLocaleString()} tasi bazadan o'chdi`
    if (res.pending > 0) {
      message.value += ` · ${res.pending.toLocaleString()} tasi terminaldan olib tashlanishini kutmoqda (sync qiling)`
    }
    checked.value = new Set()
    await load()
  } catch (e: any) {
    error.value = e.message
  }
}

// Editor kesilgan rasmni Blob sifatida beradi.
async function saveCropped(blob: Blob) {
  if (!selected.value) return

  uploading.value = true
  error.value = ''; message.value = ''
  try {
    const form = new FormData()
    form.append('photo', blob, `${selected.value.user_id}.jpg`)

    const res = await api.upload<{ person: Person; reasons: string[] }>(
      `/api/people/${selected.value.id}/photo`, form)

    selected.value = res.person
    photoBust.value++ // yangi rasmni AuthImage qayta olsin
    editing.value = false
    message.value = res.person.photo_status === 'valid'
      ? "Rasm yuklandi va tekshiruvdan o'tdi. Keyingi sync'da terminalga boradi."
      : `Rasm tekshiruvdan o'tmadi: ${res.reasons.join(', ')}`
    await load()
  } catch (e: any) {
    error.value = e.message
  } finally {
    uploading.value = false
  }
}

/**
 * Rasmni HEMIS'nikiga qaytarish.
 *
 * ⚠️ Qo'lda qo'yilgan rasm O'CHMAYDI — u tarixda qoladi, faqat terminalga
 * endi HEMIS rasmi ketadi.
 */
async function revertPhoto() {
  if (!selected.value) return
  if (!confirm(
    `${selected.value.full_name} uchun HEMIS rasmi qaytarilsinmi?\n\n` +
    `Qo'lda qo'yilgan rasm o'chmaydi, lekin terminalga endi HEMIS'niki boradi.`)) return

  error.value = ''; message.value = ''
  try {
    selected.value = await api.del<Person>(`/api/people/${selected.value.id}/photo`)
    photoBust.value++
    message.value = 'HEMIS rasmi qaytarildi — keyingi sync\'da terminalga boradi.'
    await load()
  } catch (e: any) {
    error.value = e.message
  }
}

const photoSourceLabel: Record<string, string> = {
  manual: 'Qo\'lda almashtirilgan',
  terminal: 'Terminaldan olingan',
}

watch(selected, () => { editing.value = false })

// Rasm almashtirilgach shu o'zgaradi va AuthImage qayta yuklaydi
// (URL o'sha bo'lgani uchun oddiy reaktivlik yetmaydi).
const photoBust = ref(0)
</script>

<template>
  <div v-if="message" class="alert ok">{{ message }}</div>
  <div v-if="error" class="alert err">{{ error }}</div>

  <!-- Filtr -->
  <div class="panel">
    <div class="panel-body" style="padding-bottom: 10px">
      <div class="row">
        <div style="flex: 2; min-width: 180px">
          <label>Qidiruv</label>
          <input v-model="filter.q" placeholder="Ism yoki UserID" style="width: 100%" @keyup.enter="search" />
        </div>
        <div style="flex: 1.4; min-width: 150px">
          <label>Bo'lim</label>
          <select v-model="filter.department_id" style="width: 100%" @change="search">
            <option value="">Barchasi</option>
            <option v-for="node in depTree" :key="node.dep.id" :value="String(node.dep.id)">
              {{ '— '.repeat(node.depth) }}{{ node.dep.name }} ({{ node.dep.people }})
            </option>
          </select>
        </div>
        <div style="flex: 1; min-width: 120px">
          <label>Turi</label>
          <select v-model="filter.person_type" style="width: 100%" @change="search">
            <option value="">Barchasi</option>
            <option value="employee">Xodim</option>
            <option value="student">Talaba</option>
            <option value="other">Boshqa</option>
          </select>
        </div>
        <div style="flex: 1; min-width: 110px">
          <label>Jinsi</label>
          <select v-model="filter.gender" style="width: 100%" @change="search">
            <option value="">Barchasi</option>
            <option value="Erkak">Erkak</option>
            <option value="Ayol">Ayol</option>
          </select>
        </div>
        <div style="flex: 1; min-width: 130px">
          <label>Rasm holati</label>
          <select v-model="filter.photo_status" style="width: 100%" @change="search">
            <option value="">Barchasi</option>
            <option value="valid">Yaroqli</option>
            <option value="rejected">Rad etilgan</option>
            <option value="pending">Kutilmoqda</option>
            <option value="missing">Rasm yo'q</option>
          </select>
        </div>
        <BusyButton class="primary" :busy="busy.is('search')" @click="busy.run('search', search)">
          Filtrlash
        </BusyButton>
        <button @click="showAdd = !showAdd">Odam qo'shish</button>
        <button @click="showDeps = !showDeps">Bo'limlar</button>
      </div>
    </div>
  </div>

  <PersonAddForm v-if="showAdd" :departments="depTree" @saved="showAdd = false; load()" @cancel="showAdd = false" />
  <DepartmentManager v-if="showDeps" :departments="depTree" @changed="load()" />

  <!-- Ommaviy amallar -->
  <div v-if="checked.size" class="panel">
    <div class="panel-body row" style="align-items: center">
      <strong>{{ checked.size }} ta tanlandi</strong>
      <BusyButton
        class="primary sm"
        :busy="busy.is('push')"
        :disabled="!checkedSyncable"
        busy-label="Yuborilmoqda…"
        @click="busy.run('push', pushChecked)"
      >
        Terminalga yuborish<template v-if="checkedSyncable"> ({{ checkedSyncable }})</template>
      </BusyButton>
      <BusyButton class="sm" :busy="busy.is('on')" @click="busy.run('on', () => setActive(true))">
        Faollashtirish
      </BusyButton>
      <BusyButton class="sm" :busy="busy.is('off')" @click="busy.run('off', () => setActive(false))">
        Faolsizlantirish
      </BusyButton>
      <BusyButton
        class="danger sm"
        :busy="busy.is('del')"
        busy-label="O'chirilmoqda…"
        @click="busy.run('del', removeChecked)"
      >
        O'chirish
      </BusyButton>
      <div class="spacer" />
      <BusyButton
        class="sm"
        :busy="busy.is('on-all')"
        @click="busy.run('on-all', () => setActive(true, true))"
      >
        Filtrdagi hammasini faollashtirish ({{ total.toLocaleString() }})
      </BusyButton>
      <BusyButton
        class="danger sm"
        :busy="busy.is('off-all')"
        @click="busy.run('off-all', () => setActive(false, true))"
      >
        Filtrdagi hammasini faolsizlantirish
      </BusyButton>
    </div>
  </div>

  <div
    style="display: grid; gap: 14px; align-items: start"
    :style="{ gridTemplateColumns: editing ? '1fr 480px' : '1fr 300px' }"
  >
    <div class="panel">
      <div class="panel-head">
        Odamlar
        <span class="dim" style="font-weight: 400">{{ total.toLocaleString() }} ta</span>
      </div>

      <div v-if="!loaded" class="loading-box"><span class="spinner" /> Yuklanmoqda…</div>

      <div v-else-if="!people.length" class="empty">Hech narsa topilmadi.</div>

      <table v-else>
        <thead>
          <tr>
            <th style="width: 30px"><input type="checkbox" style="min-width: auto" @change="toggleAll" /></th>
            <th style="width: 44px"></th>
            <th>Ism</th>
            <th>UserID</th>
            <th style="width: 36px"></th>
            <th>Bo'lim</th>
            <th>Lavozim / Guruh</th>
            <th>Rasm</th>
            <th>Holat</th>
          </tr>
        </thead>
        <tbody>
          <tr
            v-for="p in people"
            :key="p.id"
            :class="{ 'is-selected': selected?.id === p.id || checked.has(p.id) }"
            :style="{ opacity: p.is_active ? 1 : 0.55 }"
          >
            <td><input type="checkbox" style="min-width: auto" :checked="checked.has(p.id)" @change="toggleOne(p.id)" /></td>
            <td @click="selected = p" style="cursor: pointer">
              <AuthImage v-if="p.photo_path" :person-id="p.id" class="avatar" />
              <div v-else class="avatar" />
            </td>
            <td @click="selected = p" style="cursor: pointer">
              {{ p.full_name }}
              <span v-if="p.person_type === 'student'" class="pill info" style="margin-left: 5px">talaba</span>
            </td>
            <td class="mono">
              {{ p.user_id }}
              <div v-if="p.legacy_user_id" class="dim" style="font-size: 10.5px">eski: {{ p.legacy_user_id }}</div>
            </td>
            <td>
              <NuxtLink
                :to="`/people/${p.id}`"
                class="btn sm icon-only"
                title="To'liq profil va kunlik davomat"
                :aria-label="`${p.full_name} — to'liq profil`"
                @click.stop
              >
                <NavIcon name="eye" style="width: 15px; height: 15px" aria-hidden="true" />
              </NuxtLink>
            </td>
            <td class="dim">{{ p.department_name || '—' }}</td>
            <td class="dim">
              {{ p.staff_position || p.specialty || '—' }}
              <div v-if="p.student_group" style="font-size: 11px">
                {{ p.student_group }}<span v-if="p.level_name"> · {{ p.level_name }}</span>
              </div>
            </td>
            <td>
              <span class="pill" :class="statusPill[p.photo_status]?.cls || 'mute'">
                {{ statusPill[p.photo_status]?.label || p.photo_status }}
              </span>
            </td>
            <td>
              <span v-if="p.pending_delete" class="pill err" title="Terminaldan olib tashlanishi kutilmoqda">
                O'chirilmoqda
              </span>
              <span v-else class="pill" :class="p.is_active ? 'ok' : 'mute'">
                {{ p.is_active ? 'Faol' : 'Faolsiz' }}
              </span>
              <div v-if="p.access_status === 'temporary'" style="margin-top: 2px">
                <span class="pill warn">Vaqtincha</span>
                <div v-if="p.valid_to" class="mono dim" style="font-size: 10.5px">
                  {{ toUz(p.valid_to) }} gacha
                </div>
                <div v-else class="dim" style="font-size: 10.5px">muddatsiz</div>
              </div>
              <div class="mono dim" style="font-size: 10.5px">{{ p.synced_on }} qurilma</div>
            </td>
          </tr>
        </tbody>
      </table>

      <div class="panel-body row" style="justify-content: space-between; border-top: 1px solid var(--border-soft)">
        <span class="dim">
          {{ total ? offset + 1 : 0 }}–{{ Math.min(offset + limit, total) }} / {{ total.toLocaleString() }}
        </span>
        <div class="row" style="gap: 6px">
          <BusyButton
            class="sm"
            :busy="busy.is('prev')"
            :disabled="offset === 0"
            @click="busy.run('prev', () => page(-1))"
          >
            Oldingi
          </BusyButton>
          <BusyButton
            class="sm"
            :busy="busy.is('next')"
            :disabled="offset + limit >= total"
            @click="busy.run('next', () => page(1))"
          >
            Keyingi
          </BusyButton>
        </div>
      </div>
    </div>

    <!-- Tanlangan odam -->
    <div class="panel" style="position: sticky; top: 0">
      <div class="panel-head">{{ selected ? 'Rasm' : 'Tanlanmagan' }}</div>

      <div v-if="!selected" class="empty" style="padding: 30px 20px">Ro'yxatdan birini tanlang.</div>

      <div v-else class="panel-body">
        <AuthImage v-if="selected.photo_path" :person-id="selected.id" :bust="photoBust" class="avatar lg" />
        <div v-else class="avatar lg" style="display: grid; place-items: center; color: var(--text-faint)">
          Rasm yo'q
        </div>

        <div style="margin: 10px 0 4px; font-weight: 600">{{ selected.full_name }}</div>
        <div class="mono dim" style="font-size: 11.5px">{{ selected.user_id }}</div>
        <div class="dim" style="font-size: 11.5px; margin-top: 2px">
          {{ selected.department_name || 'Bo\'limsiz' }}
        </div>

        <div class="row" style="margin: 10px 0; gap: 5px">
          <span class="pill" :class="statusPill[selected.photo_status]?.cls || 'mute'">
            {{ statusPill[selected.photo_status]?.label || selected.photo_status }}
          </span>
          <span v-if="photoSourceLabel[selected.photo_source]" class="pill info">
            {{ photoSourceLabel[selected.photo_source] }}
          </span>
        </div>

        <div v-if="selected.photo_reject_reason" class="alert err" style="font-size: 11.5px">
          {{ selected.photo_reject_reason }}
        </div>

        <BusyButton
          v-if="!editing"
          class="primary"
          style="width: 100%; margin-top: 10px"
          :busy="uploading"
          busy-label="Tekshirilmoqda…"
          @click="editing = true"
        >
          Rasmni almashtirish
        </BusyButton>

        <PhotoEditor v-else @done="saveCropped" @cancel="editing = false" />

        <BusyButton
          v-if="!editing && selected.photo_source !== 'hemis'"
          class="sm"
          style="width: 100%; margin-top: 6px"
          :busy="busy.is('revert')"
          busy-label="Qaytarilmoqda…"
          @click="busy.run('revert', revertPhoto)"
        >
          HEMIS rasmiga qaytarish
        </BusyButton>

        <p class="dim" style="font-size: 11px; margin: 12px 0 0">
          Rasm terminal talablariga tekshiriladi: ko'zlar orasi ≥60px, sifat ≥50,
          bitta yuz. Almashtirilsa keyingi sync'da terminalga qayta yuklanadi.
        </p>
        <p v-if="selected.photo_source !== 'hemis'" class="dim" style="font-size: 11px; margin: 6px 0 0">
          Bu rasm almashtirilgan — HEMIS'dan rasm yuklash bosqichi unga tegmaydi.
        </p>
      </div>
    </div>

  </div>
</template>
