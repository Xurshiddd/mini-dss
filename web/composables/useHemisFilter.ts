/**
 * HEMIS sync tanlovi: kimni tortish va talabalar uchun qanday filtr.
 *
 * Maydon nomlari Go tomonidagi `hemis.StudentFilter` bilan bir xil —
 * ular to'g'ridan-to'g'ri `POST /api/sync/hemis` tanasiga tushadi.
 */
export type HemisStudentFilter = {
  department: number | ''
  specialty: number | ''
  group: number | ''
  curriculum: number | ''
  level: string
  education_form: string
  education_type: string
  payment_form: string
  accommodation: string
  student_status: string
  semester: string
  gender: string
  citizenship: string
  province: string
  district: string
  search: string
  passport_pin: string
  passport_number: string
  tutor_pin: string
  updated_at_from: number | ''
  updated_at_to: number | ''
}

export type HemisFilterValue = {
  employees: boolean
  students: boolean
  filter: HemisStudentFilter
}

/** Bo'sh filtr — barcha maydon bo'sh bo'lishi SHART, `reset` shunga tayanadi. */
export function emptyHemisFilter(): HemisFilterValue {
  return {
    employees: true,
    students: true,
    filter: {
      department: '', specialty: '', group: '', curriculum: '',
      level: '', education_form: '', education_type: '', payment_form: '',
      accommodation: '',
      student_status: '', semester: '', gender: '', citizenship: '',
      province: '', district: '', search: '', passport_pin: '',
      passport_number: '', tutor_pin: '', updated_at_from: '', updated_at_to: '',
    },
  }
}
