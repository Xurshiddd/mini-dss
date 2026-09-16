/**
 * Global yuklanish holati.
 *
 * `useApi` har bir so'rovda hisobni oshiradi/kamaytiradi, layout esa
 * hisob noldan katta bo'lsa sahifa ustida ingichka chiziq ko'rsatadi.
 *
 * ⚠️ Rasm yuklashlar (`blobUrl`) BU YERGA KIRMAYDI: odamlar ro'yxatida
 * o'nlab rasm parallel yuklanadi va chiziq doim yonib turardi — ya'ni
 * hech narsani anglatmay qolardi.
 */
export function useLoading() {
  const pending = useState('api-pending', () => 0)

  return {
    pending,
    active: computed(() => pending.value > 0),
    start: () => { pending.value++ },
    stop: () => { pending.value = Math.max(0, pending.value - 1) },
  }
}
