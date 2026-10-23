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

// Scale is the words of a check-in's scale, from 1 to 5.
export type Scale = readonly [string, string, string, string, string]

export function scale(...words: Scale): Scale {
  return words
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
  saving: 'Saving…',
  saved: 'Saved',
  notSaved: (why: string) => `Not saved. ${why}`,
  refused: 'The server did not take it.',
  // Amounts in their units, which every language here writes the same.
  unit: {
    kcal: (n: string) => `${n} kcal`,
    g: (n: string) => `${n} g`,
    ml: (n: string) => `${n} ml`,
  },
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
    checkIn: 'Check-in',
    mood: 'Mood',
    moods: scale('bad', 'low', 'okay', 'good', 'great'),
    hunger: 'Hunger',
    hungers: scale('full', 'calm', 'moderate', 'hungry', 'very hungry'),
    note: 'Note',
    notePlaceholder: 'restaurant, sick, travelling…',
    dayType: 'Type of day',
    dayTypes: { training: 'Training', rest: 'Rest', plan: 'By plan' },
    calibration: 'Calibration',
    counted: 'Counted in calibration.',
    leaveOut: 'Leave out of calibration',
    countAgain: 'Count it again',
    reason: 'Reason',
    // Reasons a tap writes in, the ones a day is left out for most.
    reasons: ['restaurant', 'sick', 'not logged'],
    cancel: 'Cancel',
    save: 'Save',
    delete: 'Delete',
    change: (what: string) => `Change: ${what}`,
    addMetric: 'Add a measurement',
    kind: 'Measurement',
    value: 'Value',
    notANumber: 'Write a number, such as 72.5.',
    outOfRange: 'The server does not take this value.',
  },
  meal: {
    edit: (what: string) => `Change the meal: ${what}`,
    amount: (name: string) => `Amount: ${name}`,
    remove: (name: string) => `Remove: ${name}`,
    keep: (name: string) => `Keep: ${name}`,
    removed: 'removed',
    portions: 'portions',
    save: 'Save the meal',
    cancel: 'Cancel',
    oneItem: 'A meal keeps at least one item: delete the meal instead.',
    delete: 'Delete the meal',
    confirmDelete: 'Delete this meal?',
    yesDelete: 'Delete',
    keepMeal: 'Keep it',
    refused: 'The server did not take this change.',
    // What a warning of the server's says, by its code.
    warnings: {
      atwater: 'The kcal do not match the protein, carbs and fat.',
      saturated_fat: 'Saturated fat is more than all the fat.',
      sugars: 'Sugars are more than all the carbs.',
      polyols: 'Polyols are more than all the carbs.',
      duplicate_meal:
        'A meal with the same items was logged just before: if this one was sent twice, delete it.',
      food_kept: 'A food of yours was logged with its own values, not those sent.',
    },
  },
  undo: {
    button: 'Undo the latest change',
    done: (what: string) => `Taken back: ${what}.`,
    nothing: 'There is no change to take back.',
    clash:
      'The latest change cannot be taken back: what came since clashes with it. The next undo takes back the one before.',
    // What a change taken back did, by its action.
    actions: {
      'food.save': 'a food saved',
      'food.archive': 'a food archived',
      'meal.create': 'a meal logged',
      'meal.replace': 'a meal changed',
      'meal.delete': 'a meal deleted',
      'day.exclude': 'a day left out of calibration',
      'day.include': 'a day counted again',
      'day.training': "a day's type",
      'day.checkin': 'a check-in',
      'metric.set': 'a measurement',
      'metric.delete': 'a measurement deleted',
      'metrics.merge': 'measurements imported',
      'notes.set': 'the notes for a coach',
    } as Record<string, string>,
    change: 'a change',
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
