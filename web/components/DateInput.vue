<script setup lang="ts">
/**
 * Kun/oy/yil formatidagi sana maydoni.
 *
 * KO'RSATISH uchun `<input type="date">` ISHLATILMAYDI: u formatni brauzer
 * tiliga qarab chizadi (ko'pincha oy/kun/yil) va buni boshqarib bo'lmaydi.
 * Shu sababli ko'rinadigan maydon — oddiy matn, hamma joyda kun/oy/yil.
 *
 * ⚠️ Lekin KALENDAR uchun o'sha native input yashirin holda saqlanadi va
 * faqat `showPicker()` bilan ochiladi. Ya'ni format nazorati bizda qoladi,
 * kalendar esa brauzernikini ishlatamiz — o'zimiz yozsak, u har bir tilda
 * va mobil qurilmada alohida sinovni talab qilardi.
 *
 * Tashqariga ISO (YYYY-MM-DD) chiqadi, ichkarida kun/oy/yil ko'rinadi.
 */
const props = defineProps<{ modelValue: string | null; placeholder?: string }>()
const emit = defineEmits<{ 'update:modelValue': [string | null] }>()

const text = ref(toUz(props.modelValue))
const invalid = ref(false)
const native = ref<HTMLInputElement | null>(null)

watch(() => props.modelValue, (v) => {
  // Tashqaridan o'zgarganda maydonni yangilaymiz, lekin foydalanuvchi
  // yozayotgan bo'lsa xalaqit bermaymiz.
  if (toISO(text.value) !== v) text.value = toUz(v)
})

/**
 * Yozayotganda ajratgichni O'ZI qo'yadi: "14052001" -> "14/05/2001".
 *
 * ⚠️ Faqat kursor oxirida turganda formatlaymiz. O'rtada tahrir qilinayotganda
 * ham aralashsak, kursor har bosishda oxiriga sakrab ketadi.
 */
function mask(el: HTMLInputElement) {
  if (el.selectionStart !== el.value.length) return

  const digits = el.value.replace(/\D/g, '').slice(0, 8)
  if (digits === '') return

  let out = digits.slice(0, 2)
  if (digits.length > 2) out += '/' + digits.slice(2, 4)
  if (digits.length > 4) out += '/' + digits.slice(4, 8)

  text.value = out
}

function onInput(e: Event) {
  mask(e.target as HTMLInputElement)

  // ⚠️ Yozish DAVOMIDA xato ko'rsatilmaydi: "14/0" hali tugallanmagan sana,
  // uni qizil qilish foydalanuvchini bejiz cho'chitadi. Tekshiruv `blur` da.
  invalid.value = false

  const raw = text.value.trim()
  if (raw === '') {
    emit('update:modelValue', null)
    return
  }

  const iso = toISO(raw)
  if (iso) emit('update:modelValue', iso)
}

function onBlur() {
  const raw = text.value.trim()

  if (raw === '') {
    invalid.value = false
    emit('update:modelValue', null)
    return
  }

  const iso = toISO(raw)
  invalid.value = iso === null
  if (iso) {
    text.value = toUz(iso) // "1/5/2001" -> "01/05/2001"
    emit('update:modelValue', iso)
  }
}

/** Kalendarni ochadi. Yozilgan sana bo'lsa — o'sha oydan boshlanadi. */
function openPicker() {
  const el = native.value
  if (!el) return

  el.value = toISO(text.value) || ''

  // `showPicker` bo'lmagan brauzerda oddiy fokus/klik bilan ochiladi.
  if (typeof el.showPicker === 'function') {
    try {
      el.showPicker()
      return
    } catch {
      // Ba'zi holatlarda (masalan foydalanuvchi harakatisiz) taqiqlanadi —
      // pastdagi zaxiraga tushamiz.
    }
  }

  el.focus()
  el.click()
}

function onPicked() {
  const iso = native.value?.value || ''
  if (!iso) return

  invalid.value = false
  text.value = toUz(iso)
  emit('update:modelValue', iso)
}
</script>

<template>
  <div>
    <div style="position: relative">
      <input
        v-model="text"
        type="text"
        inputmode="numeric"
        maxlength="10"
        :placeholder="placeholder || 'kk/oo/yyyy'"
        :style="{
          width: '100%',
          paddingRight: '30px',
          borderColor: invalid ? 'var(--err)' : '',
        }"
        @input="onInput"
        @blur="onBlur"
      />

      <button
        type="button"
        class="icon-only"
        title="Kalendardan tanlash"
        aria-label="Kalendardan tanlash"
        style="
          position: absolute;
          right: 1px;
          top: 1px;
          bottom: 1px;
          width: 28px;
          display: flex;
          align-items: center;
          justify-content: center;
          padding: 0;
          border: 0;
          background: transparent;
        "
        @click="openPicker"
      >
        <svg width="14" height="14" viewBox="0 0 24 24" fill="none"
             stroke="currentColor" stroke-width="2"
             stroke-linecap="round" stroke-linejoin="round">
          <rect x="3" y="4" width="18" height="18" rx="2" />
          <path d="M16 2v4M8 2v4M3 10h18" />
        </svg>
      </button>

      <!--
        ⚠️ Kalendar manbai. Ko'rinmaydi, lekin `display:none` QILINMAYDI —
        yashirilgan element uchun `showPicker()` ishlamaydi. Shuning uchun
        o'lchamsiz va shaffof qilib qo'yiladi.
      -->
      <input
        ref="native"
        type="date"
        tabindex="-1"
        aria-hidden="true"
        style="
          position: absolute;
          right: 8px;
          bottom: 0;
          width: 1px;
          height: 1px;
          padding: 0;
          border: 0;
          opacity: 0;
          pointer-events: none;
        "
        @change="onPicked"
      />
    </div>

    <p v-if="invalid" style="margin: 3px 0 0; font-size: 11px; color: var(--err)">
      Sana kun/oy/yil ko'rinishida bo'lsin — masalan 14/05/2001
    </p>
  </div>
</template>
