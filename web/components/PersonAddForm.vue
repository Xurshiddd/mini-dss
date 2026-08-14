<script setup lang="ts">
const props = defineProps<{ departments: { dep: any; depth: number }[] }>()
const emit = defineEmits<{ saved: []; cancel: [] }>()

const api = useApi()
const error = ref('')
const busy = ref(false)

const form = reactive({
  person_type: 'employee',
  user_id: '',
  full_name: '',
  department_id: '' as string,
  gender: '',
  birth_date: null as string | null,
  status: '',
  access_status: 'permanent',
  valid_to: null as string | null,
  staff_position: '',
  specialty: '',
  student_group: '',
  level_name: '',
  is_active: true,
})

async function save() {
  error.value = ''
  busy.value = true
  try {
    await api.post('/api/people', {
      ...form,
      department_id: form.department_id ? Number(form.department_id) : null,
      gender: form.gender || null,
      birth_date: form.birth_date || null,
      status: form.status || null,
      access_status: form.access_status,
      valid_to: form.access_status === 'temporary' ? form.valid_to : null,
      staff_position: form.staff_position || null,
      specialty: form.specialty || null,
      student_group: form.student_group || null,
      level_name: form.level_name || null,
    })
    emit('saved')
  } catch (e: any) {
    error.value = e.message
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <div class="panel">
    <div class="panel-head">Qo'lda odam qo'shish</div>
    <div class="panel-body" style="max-width: 720px">
      <div v-if="error" class="alert err">{{ error }}</div>

      <div class="row">
        <div class="field" style="flex: 1">
          <label>Turi</label>
          <select v-model="form.person_type" style="width: 100%">
            <option value="employee">Xodim</option>
            <option value="student">Talaba</option>
            <option value="other">Boshqa</option>
          </select>
        </div>
        <div class="field" style="flex: 1.4">
          <label>UserID — terminalga shu ID yoziladi</label>
          <input v-model="form.user_id" class="mono" placeholder="3312412112" style="width: 100%" />
        </div>
        <div class="field" style="flex: 2">
          <label>To'liq ism</label>
          <input v-model="form.full_name" placeholder="FAMILIYA ISM OTASI" style="width: 100%" />
        </div>
      </div>

      <div class="row">
        <div class="field" style="flex: 2">
          <label>Bo'lim</label>
          <select v-model="form.department_id" style="width: 100%">
            <option value="">— tanlanmagan —</option>
            <option v-for="node in props.departments" :key="node.dep.id" :value="String(node.dep.id)">
              {{ '— '.repeat(node.depth) }}{{ node.dep.name }}
            </option>
          </select>
        </div>
        <div class="field" style="flex: 1">
          <label>Jins</label>
          <select v-model="form.gender" style="width: 100%">
            <option value="">—</option>
            <option value="Erkak">Erkak</option>
            <option value="Ayol">Ayol</option>
          </select>
        </div>
        <div class="field" style="flex: 1">
          <label>Tug'ilgan sana</label>
          <DateInput v-model="form.birth_date" />
        </div>
      </div>

      <div class="row">
        <div class="field" style="flex: 1">
          <label>Kirish holati</label>
          <select v-model="form.access_status" style="width: 100%">
            <option value="permanent">Ishlamoqda</option>
            <option value="temporary">Vaqtincha</option>
          </select>
        </div>
        <div v-if="form.access_status === 'temporary'" class="field" style="flex: 1">
          <label>Muddat — bo'sh qoldirilsa cheksiz</label>
          <DateInput v-model="form.valid_to" />
        </div>
        <div v-if="form.access_status === 'temporary'" class="field" style="flex: 2">
          <p class="dim" style="margin: 0; font-size: 11.5px">
            Muddat kiritilsa, o'sha kundan keyin odam avtomatik faolsizlanadi
            va terminallarga yuborilmaydi. Kiritilmasa cheksiz turadi.
          </p>
        </div>
      </div>

      <div v-if="form.person_type === 'employee'" class="row">
        <div class="field" style="flex: 2">
          <label>Lavozim</label>
          <input v-model="form.staff_position" placeholder="Farrosh" style="width: 100%" />
        </div>
        <div class="field" style="flex: 1">
          <label>Holat</label>
          <input v-model="form.status" placeholder="Ishlamoqda" style="width: 100%" />
        </div>
      </div>

      <div v-else-if="form.person_type === 'student'" class="row">
        <div class="field" style="flex: 2">
          <label>Yo'nalish</label>
          <input v-model="form.specialty" placeholder="Yengil sanoat muhandisligi" style="width: 100%" />
        </div>
        <div class="field" style="flex: 1">
          <label>Guruh</label>
          <input v-model="form.student_group" placeholder="10-24" style="width: 100%" />
        </div>
        <div class="field" style="flex: 1">
          <label>Kurs</label>
          <input v-model="form.level_name" placeholder="2-kurs" style="width: 100%" />
        </div>
      </div>

      <label style="display: flex; align-items: center; gap: 7px; margin-bottom: 14px">
        <input v-model="form.is_active" type="checkbox" style="min-width: auto" />
        <span style="color: var(--text)">Faol — terminallarga yuboriladi</span>
      </label>

      <div class="row">
        <button class="primary" :disabled="busy" @click="save">
          {{ busy ? 'Saqlanmoqda…' : 'Qo\'shish' }}
        </button>
        <button @click="emit('cancel')">Bekor qilish</button>
      </div>

      <p class="dim" style="margin: 14px 0 0; font-size: 11.5px">
        Qo'shilgandan keyin ro'yxatdan tanlab rasm yuklang — rasmsiz odam
        terminalga yuborilmaydi (yuzsiz yozuv hech kimni kiritmaydi).
      </p>
    </div>
  </div>
</template>
