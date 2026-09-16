<script setup lang="ts">
/**
 * Amal tugmasi: bosilgach spinner chiqadi va tugma bloklanadi.
 *
 * Oddiy `<button>` o'rniga ishlatiladi — sinflar (`primary`, `sm`, `danger`)
 * o'zgarmaydi, chunki ular tashqaridan beriladi.
 */
defineProps<{
  busy?: boolean
  disabled?: boolean
  /** Ishlayotganda matn o'rniga shu ko'rsatiladi (ixtiyoriy). */
  busyLabel?: string
}>()
</script>

<template>
  <button :disabled="busy || disabled">
    <span v-if="busy" class="spinner" />
    <template v-if="busy && busyLabel">{{ busyLabel }}</template>
    <slot v-else />
  </button>
</template>
