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

/**
 * Xodim filtri — `hemis.EmployeeFilter` bilan bir xil maydon nomlari.
 *
 * Talabanikidan AYRIM: HEMIS `employee-list` ning parametrlari ham,
 * klassifikatorlari ham boshqa.
 */
export type HemisEmployeeFilter = {
  type: '' | 'all' | 'teacher' | 'employee'
  department: number | ''
  gender: string
  staff_position: string
  employee_status: string
  employment_form: string
  employment_staff: string
  employee_type: string
  academic_rank: string
  academic_degree: string
  search: string
  passport_pin: string
  passport_number: string
}

export type HemisFilterValue = {
  employees: boolean
  students: boolean
  filter: HemisStudentFilter
  employee_filter: HemisEmployeeFilter
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
    employee_filter: {
      type: '', department: '', gender: '', staff_position: '',
      employee_status: '', employment_form: '', employment_staff: '',
      employee_type: '', academic_rank: '', academic_degree: '',
      search: '', passport_pin: '', passport_number: '',
    },
  }
}

/**
 * Bo'sh maydonlarni tashlab, faqat to'ldirilgan filtrni qaytaradi.
 *
 * HEMIS bo'sh qiymatni baribir e'tiborsiz qoldiradi, lekin so'rov ham,
 * sync logidagi tanlov ham toza ko'rinsin.
 */
export function compactFilter(filter: Record<string, unknown>) {
  const out: Record<string, string | number> = {}
  for (const [k, v] of Object.entries(filter)) {
    if (v !== '' && v !== 0 && v !== null && v !== undefined) {
      out[k] = v as string | number
    }
  }
  return out
}
