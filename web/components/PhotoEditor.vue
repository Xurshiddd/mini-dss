<script setup lang="ts">
/**
 * Rasm tanlash / kameradan olish va kesish.
 *
 * ⚠️ Kesilgan rasm NATIVE o'lchamda saqlanadi (faqat juda kattasi
 * kichraytiriladi). Terminal ko'zlar orasi ≥60px talab qiladi — rasmni
 * ortiqcha kichraytirsak yuz tanilmay qoladi.
 *
 * Kamera faqat HTTPS yoki localhost'da ishlaydi (brauzer cheklovi).
 */
const emit = defineEmits<{ done: [Blob]; cancel: [] }>()

const MAX_SIDE = 1200 // kesilgandan keyingi eng katta tomon

const mode = ref<'pick' | 'camera' | 'crop'>('pick')
const error = ref('')
const busy = ref(false)

// --- manba rasm ---
const img = ref<HTMLImageElement | null>(null)
const frame = ref<HTMLElement | null>(null)
const natural = reactive({ w: 0, h: 0 })

// --- kamera ---
const video = ref<HTMLVideoElement | null>(null)
let stream: MediaStream | null = null

// --- kesish to'rtburchagi (frame ichida, foizda) ---
const crop = reactive({ x: 15, y: 8, w: 70, h: 84 })
const lockRatio = ref(true)
const RATIO = 3 / 4 // eni/bo'yi — pasport uslubidagi portret

let drag: { type: 'move' | string; sx: number; sy: number; box: typeof crop } | null = null

function loadFile(event: Event) {
  const file = (event.target as HTMLInputElement).files?.[0]
  if (!file) return
  showImage(URL.createObjectURL(file))
}

function showImage(src: string) {
  const image = new Image()
  image.onload = () => {
    natural.w = image.naturalWidth
    natural.h = image.naturalHeight
    img.value = image
    resetCrop()
    mode.value = 'crop'
  }
  image.onerror = () => (error.value = "Rasm o'qilmadi")
  image.src = src
}

function resetCrop() {
  // Boshlang'ich ramka — markazda, portret nisbatida.
  const frameRatio = natural.w / natural.h
  if (frameRatio > RATIO) {
    const w = (RATIO / frameRatio) * 100
    Object.assign(crop, { x: (100 - w) / 2, y: 2, w, h: 96 })
  } else {
    const h = (frameRatio / RATIO) * 100
    Object.assign(crop, { x: 2, y: (100 - h) / 2, w: 96, h })
  }
}

// --------------------------------------------------------------------- kamera

async function startCamera() {
  error.value = ''
  try {
    stream = await navigator.mediaDevices.getUserMedia({
      video: { width: { ideal: 1280 }, height: { ideal: 720 }, facingMode: 'user' },
    })
    mode.value = 'camera'
    await nextTick()
    if (video.value) {
      video.value.srcObject = stream
      await video.value.play()
    }
  } catch (e: any) {
    error.value = e?.name === 'NotAllowedError'
      ? 'Kameraga ruxsat berilmadi.'
      : 'Kamera ochilmadi: ' + (e?.message || e)
  }
}

function stopCamera() {
  stream?.getTracks().forEach((t) => t.stop())
  stream = null
}

function capture() {
  if (!video.value) return

  const canvas = document.createElement('canvas')
  canvas.width = video.value.videoWidth
  canvas.height = video.value.videoHeight
  canvas.getContext('2d')?.drawImage(video.value, 0, 0)

  stopCamera()
  showImage(canvas.toDataURL('image/jpeg', 0.95))
}

onUnmounted(stopCamera)

// -------------------------------------------------------------------- kesish

function onDown(event: PointerEvent, type: 'move' | string) {
  event.preventDefault()
  ;(event.target as HTMLElement).setPointerCapture?.(event.pointerId)
  drag = { type, sx: event.clientX, sy: event.clientY, box: { ...crop } }
}

function onMove(event: PointerEvent) {
  if (!drag || !frame.value) return

  const rect = frame.value.getBoundingClientRect()
  const dx = ((event.clientX - drag.sx) / rect.width) * 100
  const dy = ((event.clientY - drag.sy) / rect.height) * 100
  const b = drag.box

  if (drag.type === 'move') {
    crop.x = clamp(b.x + dx, 0, 100 - b.w)
    crop.y = clamp(b.y + dy, 0, 100 - b.h)
    return
  }

  let { x, y, w, h } = b
  if (drag.type.includes('e')) w = b.w + dx
  if (drag.type.includes('s')) h = b.h + dy
  if (drag.type.includes('w')) { x = b.x + dx; w = b.w - dx }
  if (drag.type.includes('n')) { y = b.y + dy; h = b.h - dy }

  if (lockRatio.value && frame.value) {
    // Nisbatni saqlaymiz: enini bo'yiga qarab moslaymiz (piksel hisobida).
    const px = (w / 100) * rect.width
    const targetH = px / RATIO
    h = (targetH / rect.height) * 100
    if (drag.type.includes('n')) y = b.y + b.h - h
  }

  // Juda kichik bo'lib ketmasin va ramkadan chiqmasin.
  if (w < 8 || h < 8) return
  if (x < 0 || y < 0 || x + w > 100 || y + h > 100) return

  Object.assign(crop, { x, y, w, h })
}

function onUp() { drag = null }

const clamp = (v: number, lo: number, hi: number) => Math.min(Math.max(v, lo), hi)

// Kesilgan qismning haqiqiy piksel o'lchami — foydalanuvchi ko'rib tursin.
const outSize = computed(() => {
  const w = Math.round((crop.w / 100) * natural.w)
  const h = Math.round((crop.h / 100) * natural.h)
  const scale = Math.min(1, MAX_SIDE / Math.max(w, h))
  return { w: Math.round(w * scale), h: Math.round(h * scale) }
})

// ⚠️ Terminal ko'zlar orasi ≥60px talab qiladi. Aniq o'lchash faqat
// face-api'da bo'ladi, lekin juda kichik kesilgan rasmni oldindan
// ogohlantiramiz — sync paytida rad etilgandan ko'ra shu yerda bilgani afzal.
const tooSmall = computed(() => outSize.value.w < 240)

async function confirm() {
  if (!img.value) return
  busy.value = true

  try {
    const canvas = document.createElement('canvas')
    canvas.width = outSize.value.w
    canvas.height = outSize.value.h

    const ctx = canvas.getContext('2d')!
    ctx.imageSmoothingQuality = 'high'
    ctx.drawImage(
      img.value,
      (crop.x / 100) * natural.w, (crop.y / 100) * natural.h,
      (crop.w / 100) * natural.w, (crop.h / 100) * natural.h,
      0, 0, canvas.width, canvas.height,
    )

    const blob = await new Promise<Blob | null>((resolve) =>
      canvas.toBlob(resolve, 'image/jpeg', 0.92))

    if (blob) emit('done', blob)
  } finally {
    busy.value = false
  }
}

function back() {
  stopCamera()
  img.value = null
  mode.value = 'pick'
}

const handles = ['nw', 'n', 'ne', 'e', 'se', 's', 'sw', 'w']
</script>

<template>
  <div class="editor">
    <div v-if="error" class="alert err">{{ error }}</div>

    <!-- Manba tanlash -->
    <div v-if="mode === 'pick'" class="pick">
      <label class="btn pick-btn">
        Fayldan tanlash
        <input type="file" accept="image/jpeg,image/png,image/webp" hidden @change="loadFile" />
      </label>
      <button class="pick-btn" @click="startCamera">Kameradan olish</button>
      <button class="pick-btn" @click="emit('cancel')">Bekor qilish</button>
    </div>

    <!-- Kamera -->
    <template v-else-if="mode === 'camera'">
      <video ref="video" class="stage" playsinline muted />
      <div class="row" style="margin-top: 10px">
        <button class="primary" @click="capture">Suratga olish</button>
        <button @click="back">Orqaga</button>
      </div>
    </template>

    <!-- Kesish -->
    <template v-else>
      <div
        ref="frame"
        class="stage crop-frame"
        @pointermove="onMove"
        @pointerup="onUp"
        @pointercancel="onUp"
      >
        <img :src="img?.src" alt="" draggable="false" />

        <div class="shade" :style="{
          clipPath: `polygon(0 0, 100% 0, 100% 100%, 0 100%, 0 0,
            ${crop.x}% ${crop.y}%,
            ${crop.x}% ${crop.y + crop.h}%,
            ${crop.x + crop.w}% ${crop.y + crop.h}%,
            ${crop.x + crop.w}% ${crop.y}%,
            ${crop.x}% ${crop.y}%)`
        }" />

        <div
          class="box"
          :style="{ left: crop.x + '%', top: crop.y + '%', width: crop.w + '%', height: crop.h + '%' }"
          @pointerdown="onDown($event, 'move')"
        >
          <span
            v-for="h in handles"
            :key="h"
            class="handle"
            :class="'h-' + h"
            @pointerdown.stop="onDown($event, h)"
          />
        </div>
      </div>

      <div class="row" style="margin-top: 10px; align-items: center">
        <label style="display: flex; align-items: center; gap: 6px; margin: 0">
          <input v-model="lockRatio" type="checkbox" style="min-width: auto" />
          <span style="color: var(--text)">Portret nisbati (3:4)</span>
        </label>

        <span class="mono dim">{{ outSize.w }}×{{ outSize.h }}px</span>
        <span v-if="tooSmall" class="pill warn">juda kichik</span>

        <div class="spacer" />
        <button @click="back">Orqaga</button>
        <button class="primary" :disabled="busy" @click="confirm">
          {{ busy ? 'Tayyorlanmoqda…' : 'Saqlash' }}
        </button>
      </div>

      <p v-if="tooSmall" class="dim" style="margin: 8px 0 0; font-size: 11.5px">
        Kesilgan qism juda kichik — terminal ko'zlar orasi ≥60px talab qiladi
        va bunday rasm rad etilishi mumkin. Kengroq kesing.
      </p>
    </template>
  </div>
</template>

<style scoped>
.editor { margin-top: 10px; }

.pick { display: flex; flex-direction: column; gap: 8px; }
.pick-btn { text-align: center; }

.stage {
  width: 100%;
  max-height: 420px;
  background: var(--bg);
  border: 1px solid var(--border);
  border-radius: var(--radius);
  display: block;
  object-fit: contain;
}

.crop-frame {
  position: relative;
  user-select: none;
  touch-action: none;
  overflow: hidden;
  line-height: 0;
}
.crop-frame img { width: 100%; max-height: 420px; object-fit: contain; display: block; }

.shade {
  position: absolute;
  inset: 0;
  background: rgba(0, 0, 0, 0.55);
  pointer-events: none;
}

.box {
  position: absolute;
  border: 1px solid var(--accent);
  box-shadow: 0 0 0 1px rgba(0, 0, 0, 0.35);
  cursor: move;
}
/* Uchdan bir chiziqlari — portret kadrlashda yordam beradi. */
.box::before,
.box::after {
  content: '';
  position: absolute;
  inset: 0;
  pointer-events: none;
  border-style: solid;
  border-color: rgba(255, 255, 255, 0.28);
}
.box::before { border-width: 0 1px; left: 33.33%; right: 33.33%; }
.box::after { border-width: 1px 0; top: 33.33%; bottom: 33.33%; }

.handle {
  position: absolute;
  width: 11px;
  height: 11px;
  background: var(--accent);
  border: 1px solid #fff;
  border-radius: 2px;
}
.h-nw { left: -6px; top: -6px; cursor: nwse-resize; }
.h-n  { left: calc(50% - 6px); top: -6px; cursor: ns-resize; }
.h-ne { right: -6px; top: -6px; cursor: nesw-resize; }
.h-e  { right: -6px; top: calc(50% - 6px); cursor: ew-resize; }
.h-se { right: -6px; bottom: -6px; cursor: nwse-resize; }
.h-s  { left: calc(50% - 6px); bottom: -6px; cursor: ns-resize; }
.h-sw { left: -6px; bottom: -6px; cursor: nesw-resize; }
.h-w  { left: -6px; top: calc(50% - 6px); cursor: ew-resize; }
</style>
