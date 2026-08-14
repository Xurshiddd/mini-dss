/**
 * Sana formati — HAMMA JOYDA kun/oy/yil.
 *
 * ⚠️ `<input type="date">` ISHLATILMAYDI: u formatni brauzer tiliga qarab
 * ko'rsatadi (ko'pincha oy/kun/yil) va buni CSS yoki atribut bilan
 * o'zgartirib bo'lmaydi. Shu sababli oddiy matn maydoni + qo'lda tekshiruv.
 *
 * Ichkarida (API va baza) ISO ishlatiladi: YYYY-MM-DD.
 */

const ISO = /^(\d{4})-(\d{2})-(\d{2})/
const UZ = /^(\d{1,2})[./](\d{1,2})[./](\d{4})$/

/**
 * Bugungi kun ISO ko'rinishida — LOKAL vaqt bo'yicha.
 *
 * ⚠️ `new Date().toISOString().slice(0, 10)` ISHLATILMAYDI: u sanani UTC ga
 * o'giradi. Toshkent UTC+5 bo'lgani uchun kechasi 00:00 dan 05:00 gacha u
 * KECHAGI kunni qaytaradi va hisobot bexosdan bir kun orqada ochiladi.
 */
export function todayISO(): string {
  const d = new Date()
  return [
    d.getFullYear(),
    String(d.getMonth() + 1).padStart(2, '0'),
    String(d.getDate()).padStart(2, '0'),
  ].join('-')
}

/** ISO yoki Date → "14/05/2001" */
export function toUz(value: string | Date | null | undefined): string {
  if (!value) return ''

  if (value instanceof Date) {
    return [
      String(value.getDate()).padStart(2, '0'),
      String(value.getMonth() + 1).padStart(2, '0'),
      value.getFullYear(),
    ].join('/')
  }

  const m = ISO.exec(value)
  return m ? `${m[3]}/${m[2]}/${m[1]}` : value
}

/** "14/05/2001" yoki "14.05.2001" → "2001-05-14". Yaroqsiz bo'lsa null. */
export function toISO(value: string): string | null {
  const m = UZ.exec(value.trim())
  if (!m) return null

  const [, d, mo, y] = m
  const day = Number(d)
  const month = Number(mo)
  if (month < 1 || month > 12 || day < 1 || day > 31) return null

  const iso = `${y}-${String(month).padStart(2, '0')}-${String(day).padStart(2, '0')}`

  // Haqiqiy sana ekanini tekshiramiz (31/02 kabi holatlar uchun).
  const parsed = new Date(iso + 'T00:00:00')
  if (Number.isNaN(parsed.getTime()) || parsed.getDate() !== day) return null

  return iso
}

/** Vaqt bilan: "14/05/2001 09:32" */
export function toUzDateTime(value: string | null | undefined): string {
  if (!value) return '—'
  const d = new Date(value)
  if (Number.isNaN(d.getTime())) return '—'

  return `${toUz(d)} ${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}`
}
