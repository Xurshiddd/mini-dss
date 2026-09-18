<script setup lang="ts">
// Terminalga kimni yuborish tanlovi: xodim / talaba / boshqa.
//
// Hech biri tanlanmagan bo'lsa filtr YO'Q — hamma tur yuboriladi. Shu
// sababli "hammasi" alohida holat emas, bo'sh ro'yxatning o'zi shu.
//
// ⚠️ Filtr faqat YOZISHGA ta'sir qiladi: nofaol odam tanlangan turdan
// qat'i nazar har qanday sync'da terminaldan olib tashlanadi.
const model = defineModel<string[]>({ default: () => [] })

const options = [
  { value: 'employee', label: 'Xodimlar' },
  { value: 'student', label: 'Talabalar' },
  { value: 'other', label: 'Boshqalar' },
]

function toggle(value: string) {
  model.value = model.value.includes(value)
    ? model.value.filter((v) => v !== value)
    : [...model.value, value]
}

const summary = computed(() => {
  if (!model.value.length) return 'Hamma tur yuboriladi'
  return options
    .filter((o) => model.value.includes(o.value))
    .map((o) => o.label.toLowerCase())
    .join(', ') + ' yuboriladi'
})
</script>

<template>
  <div class="type-filter">
    <div class="type-filter-head">
      <span class="type-filter-label">Kimni yuborish</span>
      <button v-if="model.length" class="type-filter-reset" @click="model = []">
        Tozalash
      </button>
    </div>
    <div class="type-chips">
      <button
        v-for="opt in options"
        :key="opt.value"
        class="type-chip"
        :class="{ on: model.includes(opt.value) }"
        :aria-pressed="model.includes(opt.value)"
        @click="toggle(opt.value)"
      >
        {{ opt.label }}
      </button>
    </div>
    <span class="type-filter-hint">{{ summary }}</span>
  </div>
</template>

<style scoped>
.type-filter { display: flex; flex-direction: column; gap: 5px; }
.type-filter-head { display: flex; align-items: baseline; justify-content: space-between; gap: 8px; }
.type-filter-label { color: var(--text-dim); font-size: 9px; text-transform: uppercase; letter-spacing: .5px; }
.type-filter-reset {
  min-width: auto;
  padding: 0;
  border: 0;
  background: none;
  color: var(--accent);
  font-size: 10px;
}
.type-chips { display: flex; flex-wrap: wrap; gap: 5px; }
.type-chip {
  min-width: auto;
  padding: 3px 9px;
  border: 1px solid var(--border);
  border-radius: 11px;
  background: var(--bg-panel-2);
  color: var(--text-dim);
  font-size: 10.5px;
}
.type-chip.on {
  border-color: color-mix(in srgb, var(--accent) 60%, var(--border));
  background: var(--accent-soft);
  color: var(--accent);
}
.type-filter-hint { color: var(--text-faint); font-size: 9.5px; }
</style>
