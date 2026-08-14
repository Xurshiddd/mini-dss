<script setup lang="ts">
// Bo'limlar ichma-ich bo'ladi: markaz → bo'lim, fakultet → kafedra.
// HEMIS'dan kelganlari `external_id` bilan, qo'lda qo'shilganlari usiz.
const props = defineProps<{ departments: { dep: any; depth: number }[] }>()
const emit = defineEmits<{ changed: [] }>()

const api = useApi()
const error = ref('')
const name = ref('')
const parentID = ref('')

async function add() {
  if (!name.value.trim()) return
  error.value = ''
  try {
    await api.post('/api/departments', {
      name: name.value.trim(),
      parent_id: parentID.value ? Number(parentID.value) : null,
    })
    name.value = ''
    parentID.value = ''
    emit('changed')
  } catch (e: any) {
    error.value = e.message
  }
}

async function remove(dep: any) {
  if (!confirm(
    `«${dep.name}» o'chirilsinmi?\n\n` +
    `Ichidagi odamlar va bo'limlar YO'QOLMAYDI — ular bo'limsiz qoladi.`)) return
  try {
    await api.del(`/api/departments/${dep.id}`)
    emit('changed')
  } catch (e: any) {
    error.value = e.message
  }
}
</script>

<template>
  <div class="panel">
    <div class="panel-head">Bo'limlar</div>
    <div class="panel-body" style="max-width: 720px">
      <div v-if="error" class="alert err">{{ error }}</div>

      <div class="row" style="margin-bottom: 14px">
        <div style="flex: 2">
          <label>Yangi bo'lim nomi</label>
          <input v-model="name" placeholder="RTT markazi" style="width: 100%" @keyup.enter="add" />
        </div>
        <div style="flex: 2">
          <label>Ichida bo'lsin — ixtiyoriy</label>
          <select v-model="parentID" style="width: 100%">
            <option value="">— yuqori daraja —</option>
            <option v-for="node in props.departments" :key="node.dep.id" :value="String(node.dep.id)">
              {{ '— '.repeat(node.depth) }}{{ node.dep.name }}
            </option>
          </select>
        </div>
        <button class="primary" @click="add">Qo'shish</button>
      </div>

      <div v-if="!props.departments.length" class="dim">
        Bo'lim yo'q. HEMIS sync'idan keyin avtomatik to'ladi yoki qo'lda qo'shing.
      </div>

      <table v-else>
        <thead>
          <tr>
            <th>Nomi</th>
            <th>Turi</th>
            <th>Odamlar</th>
            <th>Manba</th>
            <th></th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="node in props.departments" :key="node.dep.id">
            <td>
              <span class="dim">{{ '— '.repeat(node.depth) }}</span>{{ node.dep.name }}
            </td>
            <td class="dim">{{ node.dep.kind || '—' }}</td>
            <td class="mono">{{ node.dep.people }}</td>
            <td>
              <span class="pill" :class="node.dep.external_id ? 'info' : 'mute'">
                {{ node.dep.external_id ? 'HEMIS' : "qo'lda" }}
              </span>
            </td>
            <td style="text-align: right">
              <button class="danger sm" @click="remove(node.dep)">O'chirish</button>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>
