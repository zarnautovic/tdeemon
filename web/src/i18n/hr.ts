import type { Messages } from './en'
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
  retry: 'Pokušaj ponovno',
  signIn: {
    title: 'Prijava',
    email: 'Email',
    password: 'Lozinka',
    submit: 'Prijavi se',
    submitting: 'Prijava…',
    or: 'ili',
    with: (provider: string) => `Prijavi se preko ${provider}`,
    wrong: 'Pogrešan email ili lozinka.',
    tooMany: (seconds: number) =>
      `Previše neuspjelih prijava. Pokušaj ponovno za ${wait(seconds)}.`,
    busy: (seconds: number) => `Server je zauzet. Pokušaj ponovno za ${wait(seconds)}.`,
    failed: 'Prijava nije uspjela. Pokušaj ponovno.',
    noMethod: 'Ovaj server ne nudi način prijave. Javi se njegovom adminu.',
  },
  day: {
    title: 'Danas',
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
    connecting: 'Spajanje na server…',
  },
  notFound: {
    title: 'Stranica ne postoji',
    back: 'Natrag na početak',
  },
}
