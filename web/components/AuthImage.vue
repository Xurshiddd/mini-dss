<script setup lang="ts">
/**
 * Himoyalangan rasmni ko'rsatadi.
 *
 * ⚠️ Oddiy `<img :src>` token'ni URL query'ga qo'yishga majbur qilardi (u
 * loglarga tushadi). Bu komponent rasmni Authorization sarlavhasi bilan
 * blob qilib oladi — token URL'da qolmaydi.
 */
const props = defineProps<{ personId: number; bust?: string | number }>()
const api = useApi()

const src = ref('')
const failed = ref(false)

async function load() {
  // Oldingi blob'ni bo'shatamiz — aks holda xotira oqadi.
  api.revokePhoto(src.value)
  src.value = ''
  failed.value = false

  try {
    const url = await api.photoBlob(props.personId)
    if (url) src.value = url
    else failed.value = true
  } catch {
    failed.value = true
  }
}

// `bust` o'zgarsa (masalan rasm yangilangach) qayta yuklaymiz.
watch(() => [props.personId, props.bust], load)
onMounted(load)
onUnmounted(() => api.revokePhoto(src.value))
</script>

<template>
  <img v-if="src" :src="src" alt="" />
  <div v-else :class="{ 'img-failed': failed }" />
</template>
