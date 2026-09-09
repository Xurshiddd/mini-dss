<script setup lang="ts">
/**
 * Himoyalangan rasmni ko'rsatadi.
 *
 * ⚠️ Oddiy `<img :src>` token'ni URL query'ga qo'yishga majbur qilardi (u
 * loglarga tushadi). Bu komponent rasmni Authorization sarlavhasi bilan
 * blob qilib oladi — token URL'da qolmaydi.
 */
const props = defineProps<{
  /** Odam rasmi uchun. `path` berilsa e'tiborga olinmaydi. */
  personId?: number
  /** Ixtiyoriy API yo'li — masalan qoralama rasmi. */
  path?: string
  bust?: string | number
}>()
const api = useApi()

const src = ref('')
const failed = ref(false)

const target = computed(() =>
  props.path ?? (props.personId ? `/api/people/${props.personId}/photo` : ''))

async function load() {
  // Oldingi blob'ni bo'shatamiz — aks holda xotira oqadi.
  api.revokePhoto(src.value)
  src.value = ''
  failed.value = false

  if (!target.value) {
    failed.value = true
    return
  }

  try {
    const url = await api.blobUrl(target.value)
    if (url) src.value = url
    else failed.value = true
  } catch {
    failed.value = true
  }
}

// `bust` o'zgarsa (masalan rasm yangilangach) qayta yuklaymiz.
watch(() => [target.value, props.bust], load)
onMounted(load)
onUnmounted(() => api.revokePhoto(src.value))
</script>

<template>
  <img v-if="src" :src="src" alt="" />
  <div v-else :class="{ 'img-failed': failed }" />
</template>
