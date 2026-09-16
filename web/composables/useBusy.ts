/**
 * Tugma bosilganda "ishlayapti" holati.
 *
 * Bitta sahifada bir nechta amal bo'ladi, shuning uchun holat KALIT bilan
 * saqlanadi: qaysi tugma bosilgan bo'lsa, o'shanda spinner chiqadi.
 *
 * ⚠️ Amal ketayotganda o'sha tugma qayta bosilmaydi — takroriy so'rov
 * terminalga ikki marta yozishga urinishga olib kelardi.
 */
export function useBusy() {
  const busy = ref<string>('')

  async function run<T>(key: string, fn: () => Promise<T>): Promise<T | undefined> {
    if (busy.value === key) return
    busy.value = key
    try {
      return await fn()
    } finally {
      busy.value = ''
    }
  }

  return {
    busy,
    run,
    is: (key: string) => busy.value === key,
    any: computed(() => busy.value !== ''),
  }
}
