import { plural } from './plural'

// The app's text in English, the base language. Every other language has
// the same keys (Messages), which the type checker holds it to; a text
// with values in it is a function of them.

// wait says how long, rounded up to whole minutes from a minute on.
function wait(seconds: number): string {
  if (seconds < 60) {
    return `${String(seconds)} ${plural('en', seconds, { one: 'second', other: 'seconds' })}`
  }
  const minutes = Math.ceil(seconds / 60)
  return `${String(minutes)} ${plural('en', minutes, { one: 'minute', other: 'minutes' })}`
}

export const en = {
  nav: {
    label: 'Sections',
    day: 'Day',
    foods: 'Foods',
    charts: 'Charts',
    settings: 'Settings',
  },
  soon: 'This part is on its way.',
  unreachable: 'The server cannot be reached.',
  serverFailed: 'The server failed to answer.',
  failed: 'Something went wrong.',
  connecting: 'Connecting to the server…',
  retry: 'Try again',
  signIn: {
    title: 'Sign in',
    email: 'Email',
    password: 'Password',
    submit: 'Sign in',
    submitting: 'Signing in…',
    or: 'or',
    with: (provider: string) => `Sign in with ${provider}`,
    wrong: 'Wrong email or password.',
    tooMany: (seconds: number) => `Too many failed sign-ins. Try again in ${wait(seconds)}.`,
    busy: (seconds: number) => `The server is busy. Try again in ${wait(seconds)}.`,
    failed: 'Signing in failed. Try again.',
    noMethod: 'This server offers no way to sign in. Ask its admin.',
  },
  loading: 'Loading…',
  day: {
    today: 'Today',
    previous: 'The day before',
    next: 'The day after',
    toToday: 'Back to today',
    training: 'Training day',
    rest: 'Rest day',
    excluded: (reason: string) => (reason ? `Out of calibration: ${reason}` : 'Out of calibration'),
    eaten: 'Eaten',
    ofTarget: (kcal: string) => `of ${kcal} kcal`,
    left: (kcal: string) => `${kcal} kcal left`,
    over: (kcal: string) => `${kcal} kcal over`,
    noTarget: 'No target yet: the plan sets one.',
    protein: 'Protein',
    carbs: 'Carbs',
    fat: 'Fat',
    // The macros in a line, short: P 88 · C 146 · F 41.
    short: { protein: 'P', carbs: 'C', fat: 'F' },
    meals: 'Meals',
    noMeals: 'Nothing logged yet.',
    portions: (n: string, count: number) =>
      `${n} ${plural('en', count, { one: 'portion', other: 'portions' })}`,
    measurements: 'Measurements',
    noMeasurements: 'None yet.',
    metric: {
      weight: 'Weight',
      body_fat: 'Body fat',
      lean_mass: 'Lean mass',
      steps: 'Steps',
      active_kcal: 'Active energy',
      basal_kcal: 'Resting energy',
      sleep_h: 'Sleep',
      rhr: 'Resting heart rate',
      hrv: 'Heart rate variability',
      vo2max: 'VO₂ max',
      exercise_min: 'Exercise',
    },
  },
  foods: {
    title: 'Foods',
  },
  charts: {
    title: 'Charts',
  },
  settings: {
    title: 'Settings',
    account: 'Account',
    signedInAs: (name: string, email: string) => `Signed in as ${name} (${email}).`,
    signOut: 'Sign out',
    signOutFailed: 'Signing out failed. Try again.',
    about: 'About',
  },
  notFound: {
    title: 'Page not found',
    back: 'Back to the start',
  },
}

export type Messages = typeof en
