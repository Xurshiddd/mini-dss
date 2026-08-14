<script setup lang="ts">
/**
 * Kun/oy/yil formatidagi sana maydoni.
 *
 * `<input type="date">` o'rniga ishlatiladi, chunki u formatni brauzer
 * tiliga qarab ko'rsatadi va biz uni boshqara olmaymiz.
 *
 * Tashqariga ISO (YYYY-MM-DD) chiqadi, ichkarida kun/oy/yil ko'rinadi.
 */
const props = defineProps<{ modelValue: string | null; placeholder?: string }>()
const emit = defineEmits<{ 'update:modelValue': [string | null] }>()

const text = ref(toUz(props.modelValue))
const invalid = ref(false)

watch(() => props.modelValue, (v) => {
  // Tashqaridan o'zgarganda maydonni yangilaymiz, lekin foydalanuvchi
  // yozayotgan bo'lsa xalaqit bermaymiz.
  if (toISO(text.value) !== v) text.value = toUz(v)
})

function onInput() {
  const raw = text.value.trim()

  if (raw === '') {
    invalid.value = false
    emit('update:modelValue', null)
    return
  }

  const iso = toISO(raw)
  invalid.value = iso === null
  if (iso) emit('update:modelValue', iso)
}
</script>

<template>
  <div>
    <input
      v-model="text"
      type="text"
      inputmode="numeric"
      maxlength="10"
      :placeholder="placeholder || 'kk/oo/yyyy'"
      :style="{ width: '100%', borderColor: invalid ? 'var(--err)' : '' }"
      @input="onInput"
      @blur="onInput"
    />
    <p v-if="invalid" style="margin: 3px 0 0; font-size: 11px; color: var(--err)">
      Sana kun/oy/yil ko'rinishida bo'lsin — masalan 14/05/2001
    </p>
  </div>
</template>
