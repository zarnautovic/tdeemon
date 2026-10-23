import { scale, type Messages } from './en'
import { plural } from './plural'

// wait says how long, rounded up to whole minutes from a minute on.
function wait(seconds: number): string {
  if (seconds < 60) {
    return `${String(seconds)} ${plural('hr', seconds, { one: 'sekundu', few: 'sekunde', other: 'sekundi' })}`
  }
  const minutes = Math.ceil(seconds / 60)
  return `${String(minutes)} ${plural('hr', minutes, { one: 'minutu', few: 'minute', other: 'minuta' })}`
}

export const hr: Messages = {
  nav: {
    label: 'Odjeljci',
    day: 'Dan',
    foods: 'Hrana',
    charts: 'Grafovi',
    settings: 'Postavke',
  },
  soon: 'Ovaj dio stiže uskoro.',
  unreachable: 'Server nije dostupan.',
  serverFailed: 'Server nije uspio odgovoriti.',
  failed: 'Nešto je pošlo po zlu.',
  connecting: 'Spajanje na server…',
  retry: 'Pokušaj ponovno',
  signIn: {
    title: 'Prijava',
    email: 'Email',
    password: 'Lozinka',
    submit: 'Prijavi se',
    submitting: 'Prijava…',
    or: 'ili',
    with: (provider: string) => `Prijavi se putem usluge ${provider}`,
    wrong: 'Pogrešan email ili lozinka.',
    tooMany: (seconds: number) =>
      `Previše neuspjelih prijava. Pokušaj ponovno za ${wait(seconds)}.`,
    busy: (seconds: number) => `Server je zauzet. Pokušaj ponovno za ${wait(seconds)}.`,
    failed: 'Prijava nije uspjela. Pokušaj ponovno.',
    noMethod: 'Ovaj server ne nudi način prijave. Javi se njegovom adminu.',
  },
  loading: 'Učitavanje…',
  saving: 'Spremanje…',
  saved: 'Spremljeno',
  notSaved: (why: string) => `Nije spremljeno. ${why}`,
  // Amounts in their units, which every language here writes the same.
  unit: {
    kcal: (n: string) => `${n} kcal`,
    g: (n: string) => `${n} g`,
    ml: (n: string) => `${n} ml`,
  },
  day: {
    today: 'Danas',
    previous: 'Dan prije',
    next: 'Dan poslije',
    toToday: 'Natrag na danas',
    training: 'Dan treninga',
    rest: 'Dan odmora',
    excluded: (reason: string) => (reason ? `Izvan kalibracije: ${reason}` : 'Izvan kalibracije'),
    eaten: 'Pojedeno',
    ofTarget: (kcal: string) => `od ${kcal} kcal`,
    left: (kcal: string) => `još ${kcal} kcal`,
    over: (kcal: string) => `${kcal} kcal previše`,
    noTarget: 'Još nema cilja: postavlja ga plan.',
    protein: 'Proteini',
    carbs: 'Ugljikohidrati',
    fat: 'Masti',
    short: { protein: 'P', carbs: 'UH', fat: 'M' },
    meals: 'Obroci',
    noMeals: 'Još ništa nije upisano.',
    portions: (n: string, count: number) =>
      `${n} ${plural('hr', count, { one: 'porcija', few: 'porcije', other: 'porcija' })}`,
    measurements: 'Mjerenja',
    noMeasurements: 'Još nema mjerenja.',
    metric: {
      weight: 'Kilaža',
      body_fat: 'Masno tkivo',
      lean_mass: 'Nemasna masa',
      steps: 'Koraci',
      active_kcal: 'Aktivna energija',
      basal_kcal: 'Energija u mirovanju',
      sleep_h: 'San',
      rhr: 'Puls u mirovanju',
      hrv: 'Varijabilnost pulsa',
      vo2max: 'VO₂ max',
      exercise_min: 'Vježbanje',
    },
    checkIn: 'Check-in',
    mood: 'Osjećaj',
    moods: scale('loše', 'slabije', 'solidno', 'dobro', 'odlično'),
    hunger: 'Glad',
    hungers: scale('sit', 'mirno', 'umjereno', 'gladan', 'stalno gladan'),
    note: 'Napomena',
    notePlaceholder: 'restoran, bolestan, putovanje…',
    dayType: 'Tip dana',
    dayTypes: { training: 'Trening', rest: 'Odmor', plan: 'Po planu' },
    calibration: 'Kalibracija',
    counted: 'Dan ulazi u kalibraciju.',
    leaveOut: 'Izostavi iz kalibracije',
    countAgain: 'Vrati u kalibraciju',
    reason: 'Razlog',
    reasons: ['restoran', 'bolestan', 'nije sve upisano'],
    cancel: 'Odustani',
    save: 'Spremi',
    delete: 'Obriši',
    change: (what: string) => `Promijeni: ${what}`,
    addMetric: 'Dodaj mjerenje',
    kind: 'Mjerenje',
    value: 'Vrijednost',
    notANumber: 'Upiši broj, npr. 72,5.',
    outOfRange: 'Server ne prima ovu vrijednost.',
  },
  foods: {
    title: 'Hrana',
  },
  charts: {
    title: 'Grafovi',
  },
  settings: {
    title: 'Postavke',
    account: 'Račun',
    signedInAs: (name: string, email: string) => `Prijavljeni račun: ${name} (${email}).`,
    signOut: 'Odjavi se',
    signOutFailed: 'Odjava nije uspjela. Pokušaj ponovno.',
    about: 'O aplikaciji',
  },
  notFound: {
    title: 'Stranica ne postoji',
    back: 'Natrag na početak',
  },
}
