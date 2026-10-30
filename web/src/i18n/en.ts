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
    today: 'Today',
    foods: 'Foods',
    progress: 'Progress',
    plan: 'Plan',
    settings: 'Settings',
    logMeal: 'Log a meal',
    darkTheme: 'Dark theme',
    // Describes the plan's section while a proposal waits there.
    waits: 'A proposal waits',
    // The keyboard's first stop, past the sections to the page.
    skip: 'Skip to content',
  },
  toast: {
    close: 'Close',
    // The region of the toasts, which F6 takes the focus to.
    region: 'Notifications',
    undo: 'Undo',
    see: (day: string) => `See ${day}`,
    aMeal: 'a meal',
    logged: (what: string, kcal: string) => `Logged: ${what} · ${kcal}`,
    changed: (what: string) => `Changed: ${what}`,
    moved: (day: string) => `Moved to ${day}`,
    movedToday: 'Moved to today',
    deleted: (what: string) => `Deleted: ${what}`,
    checkIn: 'Check-in saved',
    dayNote: 'Note saved',
    dayType: 'Day type saved',
    excluded: 'Day left out of calibration',
    included: 'Day back in calibration',
    saved: 'Saved',
    savedOf: (what: string) => `Saved: ${what}`,
    archived: (food: string) => `Archived: ${food}`,
    inUse: (food: string) => `In use again: ${food}`,
    notes: 'Notes for your AI coach saved',
  },
  update: {
    // A toast when a new version of the app is ready, and its button.
    ready: 'New version ready',
    reload: 'Reload the app',
  },
  shortcuts: {
    title: 'Keyboard shortcuts',
    logMeal: 'Log a meal',
    find: 'Find a food',
    days: 'The day before or after',
    list: 'These shortcuts',
    toasts: 'The notifications',
    close: 'Close',
    undo: 'Undo the latest change',
    // A section, a key after G: "G then D".
    andThen: 'then',
    sections: {
      today: 'Today',
      foods: 'Foods',
      progress: 'Progress',
      plan: 'Plan',
      settings: 'Settings',
    },
    // The groups of the list, and the keys of a meal of the day's.
    anywhere: 'Anywhere',
    goTo: 'Go to a section',
    onDay: 'On a day',
    meals: 'The meal before or after',
    change: 'Change the meal',
    repeat: 'Repeat the meal today',
    remove: 'Delete the meal',
  },
  soon: 'This part is on its way.',
  unreachable: 'The server cannot be reached.',
  serverFailed: 'The server failed to answer.',
  failed: 'Something went wrong.',
  connecting: 'Connecting to the server…',
  retry: 'Try again',
  close: 'Close',
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
    // What tdeemon is, under its name.
    tagline: 'Log what you eat; tdeemon learns what you burn, and your AI coach plans from it.',
  },
  loading: 'Loading…',
  // What did not load, said over why, in its place on a page whose
  // header and navigation stay.
  loadFailed: {
    today: "Couldn't load today.",
    day: "Couldn't load this day.",
    foods: "Couldn't load your foods.",
    food: "Couldn't load this food.",
    progress: "Couldn't load your progress.",
    plan: "Couldn't load your plan.",
    profile: "Couldn't load your profile.",
    limits: "Couldn't load the limits of your plan.",
    notes: "Couldn't load the notes for your AI coach.",
    keys: "Couldn't load your access keys.",
  },
  // A thin bar over what a page shows when asking for it again failed:
  // the server out of reach (offline) or failing, and how old it is.
  stale: {
    offline: (when: string) => `Offline · data from ${when}`,
    failed: (when: string) => `Not updated · data from ${when}`,
    // Of a day before today: its date and time.
    dayTime: (day: string, time: string) => `${day}, ${time}`,
  },
  notSaved: (why: string) => `Not saved. ${why}`,
  refused: 'The server did not take it.',
  // Amounts in their units, which every language here writes the same.
  unit: {
    kcal: (n: string) => `${n} kcal`,
    g: (n: string) => `${n} g`,
    ml: (n: string) => `${n} ml`,
    // Hours and minutes, the minutes in two digits: 6 h 52.
    hm: (hours: string, minutes: string) => `${hours} h ${minutes}`,
  },
  // Calibration on its way to a TDEE it trusts (DESIGN.md §7): what it
  // counts against what it needs, both the server's, and when a first
  // TDEE may come.
  calibration: {
    title: 'Calibration',
    lead: 'Your TDEE is measured once there are enough days, meals and weigh-ins.',
    days: 'Days',
    loggedDays: 'Logged days',
    weighIns: 'Weigh-ins',
    // So many of the fewest it needs: 9 of 14; short, 9/14.
    of: (has: string, need: string) => `${has} of ${need}`,
    ofShort: (has: string, need: string) => `${has}/${need}`,
    // Beside a count that has what it needs, for a screen reader.
    met: 'enough',
    // The first weigh-in to the last, which need days between them.
    apart: (has: string, need: string) => `${has} of ${need} days from the first to the last`,
    expected: (date: string) => `First TDEE around ${date} if you keep this up.`,
    // Without a day foreseen: no meal or no weigh-in lately.
    start: 'Log your meals and weigh in, and the counts grow day by day.',
    // A TDEE measured but not reliable yet, and what it needs to be.
    unsure: (kcal: string, half: string) => `TDEE ≈ ${kcal} ± ${half} · not reliable yet.`,
    doubts: {
      wide_interval: (half: string, weighIns: string) =>
        `Reliable once ± is ${half} or less. Weighing in ${weighIns}× a week helps most.`,
      few_weigh_ins: (weighIns: string, count: number) =>
        `It needs ${weighIns} ${plural('en', count, { one: 'weigh-in', other: 'weigh-ins' })} a week, in the last week too.`,
      few_logged_days: (days: string, count: number) =>
        `It needs ${days} logged ${plural('en', count, { one: 'day', other: 'days' })} a week, in the last week too.`,
    },
    beforePhase: (date: string) =>
      `New phase. Until ${date} the TDEE comes from the days before it.`,
  },
  // What a number is, an ⓘ beside it away (InfoTip): the ⓘ's name, the
  // number's, and two or three sentences, from DESIGN.md §7 in plain
  // words.
  info: {
    tdee: {
      label: 'What TDEE is',
      title: 'TDEE',
      text: 'TDEE is how much energy you burn a day. It is measured from what you log and how your trend weight moved over the last 6 weeks. ± says how sure that is: with 80% probability, your real TDEE is within that range.',
    },
    trusted: {
      label: 'What reliable means',
      title: 'Reliable',
      text: 'Reliable means the ± is 200 kcal or less. It also needs 4 logged days and 3 weigh-ins every week, the last one too. Until then TDEE is a first estimate, and the phase gets no verdict.',
    },
    trend: {
      label: 'What trend weight is',
      title: 'Trend weight',
      text: 'Trend weight is your weight without the day-to-day swings of water and salt. Each weigh-in moves it a tenth of the way, so it follows the direction, not a single day. It lags the scale by about 9 days.',
    },
    left: {
      label: 'What left to TDEE is',
      title: 'Left to TDEE',
      text: 'TDEE less what was eaten that day: how much more you can eat without eating more than you burn. Past it, the day is a surplus. The plan works on the weekly average, so one day over changes little.',
    },
    deficit: {
      label: 'What the planned deficit is',
      title: 'Planned deficit',
      text: 'How far the weekly average of your targets is under your TDEE. The bigger it is, the faster your weight goes down; a surplus, the average over TDEE, makes it go up. A deficit of about 500 kcal a day is about half a kilo a week.',
    },
    verdict: {
      label: 'How the phase is judged',
      title: 'The verdict',
      text: "The verdict compares how fast your trend weight really changes with the phase's range. It comes once TDEE is reliable and measured from the phase's own days. Only then does it propose a new target.",
    },
  },
  day: {
    today: 'Today',
    previous: 'The day before',
    next: 'The day after',
    toToday: 'Back to today',
    // The morning summary, as its notification sends it.
    morning: {
      title: 'Morning summary',
      open: 'Morning summary',
      failed: 'The morning summary did not load',
    },
    training: 'Training day',
    rest: 'Rest day',
    excluded: (reason: string) => (reason ? `Out of calibration: ${reason}` : 'Out of calibration'),
    eaten: 'Eaten',
    left: (kcal: string) => `${kcal} kcal left`,
    summary: {
      title: 'Energy and macros',
      left: 'kcal left',
      // Past the target by more than the week takes in its stride; on
      // it, within that either way; under it, of a day that is over.
      over: 'over target',
      on: 'on target',
      under: 'under target',
      ring: (eaten: string, target: string) => `Eaten ${eaten} of ${target} kcal`,
      ringLeft: (kcal: string) => `${kcal} left`,
      ringOver: (kcal: string) => `${kcal} over target`,
      // How far from it, signed: on target (+60).
      ringOn: (signed: string) => `on target (${signed})`,
      ringUnder: (kcal: string) => `${kcal} under target`,
      target: 'Target',
      tdee: 'TDEE',
      // The TDEE less what was eaten: what is left of it today, how far a
      // day before stayed under it, or how far either went over it.
      leftToTdee: 'Left to TDEE',
      underTdee: 'Under TDEE',
      overTdee: 'Over TDEE',
      // A day before that went over its target, said beside the TDEE.
      overUnderTdee: (over: string, under: string) =>
        `${over} over target, still ${under} under TDEE.`,
      overOverTdee: (over: string, tdee: string) => `${over} over target and ${tdee} over TDEE.`,
      // After the grams eaten, which are shown bolder: 112 / 160 g.
      ofTarget: (target: string) => `/ ${target} g`,
      toGo: (g: string) => `${g} to go`,
      // Protein's target is a floor: reached, and by how much past it.
      reached: 'reached',
      reachedBy: (g: string) => `reached · +${g} g`,
      // Carbs or fat past their target: how far, nothing more.
      overBy: (g: string) => `+${g} g`,
      noTarget: 'A plan gives the day its target.',
      setPlan: 'Set up a plan',
      // A day before the plan's first target, which says when it starts;
      // a plan of phases that has no target yet.
      startsOn: (date: string) => `No target on this day. The plan starts on ${date}.`,
      noTargetYet: 'The plan gives no target yet.',
      seePlan: 'See the plan',
      weekOf: (n: number, of: number) => `week ${String(n)} of ${String(of)}`,
      week: (n: number) => `week ${String(n)}`,
      rate: (pct: string) => `${pct}%/wk`,
      rates: { none: '', on_target: 'in range', below: 'below range', above: 'above range' },
    },
    week: {
      label: 'The week',
      day: (date: string, said: string) => `${date}: ${said}`,
      eatenOf: (eaten: string, target: string) => `${eaten} of ${target} kcal`,
      eaten: (eaten: string) => `${eaten} kcal`,
      nothing: 'nothing logged',
      toCome: 'to come',
      // The week's line: what the days logged ate a day on average,
      // against their target. Today, still open, is not counted.
      thisWeek: 'This week',
      thatWeek: 'That week',
      // Before the average, which is shown bolder: "This week: average 1,905".
      average: (week: string) => `${week}: average`,
      of: (target: string) => `of ${target}`,
      under: (kcal: string) => `${kcal} under a day`,
      over: (kcal: string) => `${kcal} over a day`,
      on: 'on target',
      none: (week: string) => `${week}: nothing to average yet`,
      logged: (n: number) =>
        `${String(n)} ${plural('en', n, { one: 'day', other: 'days' })} logged`,
      span: (from: string, to: string) => `${from} – ${to}`,
      // The week's days in a sheet, and what the days left can eat.
      days: "The week's days",
      dayColumn: 'Day',
      eatenColumn: 'Eaten',
      targetColumn: 'Target',
      today: 'today, counted once over',
      budget: (kcal: string) =>
        `Until Sunday you can average ${kcal} a day and stay on your weekly target.`,
      budgetToday: (kcal: string) => `Today you can eat ${kcal} and stay on your weekly target.`,
      // Past the plan's kcal floor: the day's target, not less.
      keep: (kcal: string) =>
        `The week is ${kcal} over target. Stick to your daily target until Sunday.`,
      keepToday: (kcal: string) => `The week is ${kcal} over target. Stick to today's target.`,
      legendTarget: 'target',
      legendAverage: 'average',
    },
    // A desktop's meals, in a table.
    columns: { time: 'Time', meal: 'Meal', kcal: 'kcal', total: 'Total' },
    phase: 'Phase',
    dayTypeOf: (type: string) => `Type of day: ${type}`,
    protein: 'Protein',
    carbs: 'Carbs',
    fat: 'Fat',
    // The macros in a line, short: P 88 · C 146 · F 41.
    short: { protein: 'P', carbs: 'C', fat: 'F' },
    meals: 'Meals',
    mealsTotal: (n: number, kcal: string) => `${String(n)} · ${kcal} kcal`,
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
    // The check-in in a line, while the user has said nothing yet.
    checkInAsk: 'How was today?',
    checkInAdd: 'Add',
    checkInAddOf: 'Add a check-in',
    savedAsYouGo: 'saved as you go',
    addNote: 'Add a note',
    mood: 'Mood',
    moods: scale('bad', 'low', 'okay', 'good', 'great'),
    hunger: 'Hunger',
    hungers: scale('full', 'a bit hungry', 'hungry', 'very hungry', 'starving'),
    note: 'Note',
    notePlaceholder: 'restaurant, sick, travelling…',
    dayTypes: { training: 'Training', rest: 'Rest', plan: 'By plan' },
    planDecided: { training: 'training', rest: 'rest' },
    calibration: 'Calibration',
    inCalibration: 'Counted in calibration',
    calibrationHint:
      "Calibration measures your TDEE from the days it counts. Leave out a day you couldn't log well.",
    leaveOut: 'Leave out of calibration',
    body: {
      title: 'Body today',
      // Of a day that is not today.
      titleOfDay: 'Body',
      all: 'All measurements',
      trend: (kg: string) => `trend ${kg} kg`,
      none: '—',
      // In place of the tiles, while the user has measured nothing yet.
      addWeight: 'Add a weigh-in',
    },
    weighIn: {
      // The line of today's hero in the morning, while it has no weigh-in.
      morning: 'Morning weigh-in?',
      save: 'Save the weigh-in',
      // The quick add's chip.
      chip: 'Weigh-in',
    },
    reason: 'Reason',
    // Reasons a tap writes in, the ones a day is left out for most: each
    // stored under its code, the server's words, and said in the
    // language.
    reasons: { restaurant: 'restaurant', sick: 'sick', 'not logged': 'not logged' },
    cancel: 'Cancel',
    save: 'Save',
    delete: 'Delete',
    change: (what: string) => `Change: ${what}`,
    addMetric: 'Add a measurement',
    kind: 'Measurement',
    value: 'Value',
    notANumber: 'Write a number, such as 72.5.',
    outOfRange: 'The server does not take this value.',
    between: (least: string, most: string, unit: string) =>
      `Give a value from ${least} to ${most}${unit ? ` ${unit}` : ''}.`,
  },
  meal: {
    edit: (what: string) => `Change the meal: ${what}`,
    // The visible words of the buttons named above and below.
    change: 'Change',
    repeatToday: 'Repeat today',
    deleteShort: 'Delete',
    deleteOf: (what: string) => `Delete: ${what}`,
    repeat: (what: string) => `Repeat today: ${what}`,
    slots: { breakfast: 'Breakfast', lunch: 'Lunch', snack: 'Snack', dinner: 'Dinner' },
    // A meal without a name of its own is named by its items: "Skyr and
    // banana", "Skyr + 2".
    both: (first: string, second: string) => `${first} and ${second}`,
    andMore: (first: string, more: number) => `${first} + ${String(more)}`,
    items: (n: number) => `${String(n)} ${plural('en', n, { one: 'item', other: 'items' })}`,
    ai: 'AI',
    estimate: 'estimate',
    repeated: (what: string, kcal: string) => `Repeated: ${what} · ${kcal}`,
    seeToday: 'See it today',
    amount: (name: string) => `Amount: ${name}`,
    remove: (name: string) => `Remove: ${name}`,
    keep: (name: string) => `Keep: ${name}`,
    removed: 'removed',
    portions: (n: number) => plural('en', n, { one: 'portion', other: 'portions' }),
    save: 'Save the meal',
    cancel: 'Cancel',
    oneItem: 'A meal needs at least one item. Delete the meal instead.',
    delete: 'Delete the meal',
    refused: 'The server did not take this change.',
    recomputed: "A changed amount uses the food's current values.",
    note: 'Note',
    add: 'Add to the meal',
    addNote: 'Add a note',
    changeNote: 'Change the note',
    day: 'Day',
    yesterday: 'Yesterday',
    timeToCome: "That time hasn't come yet. Log a meal once it's eaten.",
    anotherFood: (name: string) => `Another food in place of: ${name}`,
    // An item's menu of the ways to change it but its amount, and the
    // visible words of its choices, named in full above.
    more: (name: string) => `More: ${name}`,
    anotherFoodShort: 'Another food',
    removeShort: 'Remove',
    inPlaceOf: (name: string) => `In place of ${name}:`,
    findInPlaceOf: (name: string) => `A food of yours in place of ${name}`,
    saveAsFood: (name: string) => `Save as a food of yours: ${name}`,
    asFood: 'Saved as your food, one portion eaten.',
    asFoodShort: 'Save as a food',
    editTitle: 'Change the meal',
    itemsTitle: 'Items',
    // The meal's total as changed: "+42 from the changes".
    fromChanges: (kcal: string) => `${kcal} kcal from the changes`,
    was: (kcal: string) => `was ${kcal}`,
    // A time typed that is none, and how one is: 7:30 PM, or 19:30.
    timeFormat: (example: string) => `Type the time as ${example}.`,
    // The item the one-off import of history gave the rest of a meal's total.
    restOfMeal: 'Rest of the meal',
    estimateValue: { kcal: 'kcal', protein: 'P (g)', carbs: 'C (g)', fat: 'F (g)' },
    value: (what: string, name: string) => `${what}: ${name}`,
    changedElsewhere: 'Changed elsewhere since this opened.',
    // What a warning of the server's says, by its code.
    warnings: {
      atwater: 'The kcal do not match the protein, carbs and fat.',
      saturated_fat: 'Saturated fat is more than all the fat.',
      sugars: 'Sugars are more than all the carbs.',
      polyols: 'Polyols are more than all the carbs.',
      duplicate_meal:
        'A meal with the same items was logged just before. If this one was sent twice, delete it.',
      food_kept: 'A food of yours was logged with its own values, not those sent.',
    },
  },
  scan: {
    // A barcode's step of Log a meal: the camera, or the digits typed.
    title: 'Scan a barcode',
    camera: 'The camera',
    starting: 'Starting the camera…',
    holdStill: 'Hold the barcode in the frame.',
    refused: 'The camera is not allowed here. Type the digits instead.',
    noCamera: 'No camera here. Type the digits instead.',
    typeIt: 'Or type the digits under the bars',
    lookUp: 'Look up the code',
    digits: '8, 12, 13 or 14 digits.',
    check: 'Those digits do not add up: check them.',
    broken: 'The barcode reader did not load. Type the digits instead.',
    makeIt: 'Make the food',
    looking: (code: string) => `Looking up ${code}…`,
    unknown: (code: string) =>
      `Nobody has ${code} yet. Type its values off the label, and it is yours next time.`,
    misread: (code: string) => `${code} does not add up: a digit was misread. Scan it again.`,
    failed: (why: string) => `The lookup failed: ${why}`,
    again: 'Scan it again',
    fromOpenFoodFacts: 'Found at',
    checkLabel: 'Check the values against the label before you add it.',
    yours: 'Your food',
    named: (name: string) => `It is your ${name}, without this barcode yet.`,
    link: 'Add the barcode to it',
  },
  log: {
    open: 'Log a meal',
    add: {
      breakfast: 'Add breakfast',
      lunch: 'Add lunch',
      snack: 'Add a snack',
      dinner: 'Add dinner',
    },
    left: (kcal: string) => `${kcal} kcal left`,
    title: 'New meal',
    // When the meal is: "Snack · today at 16:10".
    when: (slot: string, day: string, time: string) => `${slot} · ${day} at ${time}`,
    today: 'today',
    change: 'Change',
    changeOf: 'Change the name and time',
    description: 'What it was',
    descriptionHint: 'Breakfast, a snack…',
    time: 'Time',
    nightAfter: (time: string) => `${time} is the night after this day.`,
    items: 'What was in it',
    find: 'Find a food of yours',
    findHint: 'A name, a brand or another name',
    search: 'Find a food or a meal',
    noFood: 'No food of yours matches. Add it as an estimate.',
    nothing: 'Nothing matches. Add an estimate, or make a new food.',
    none: 'Nothing added yet. Find a food, or add an estimate.',
    lists: 'Show',
    list: { all: 'All', recent: 'Recent', frequent: 'Frequent', meals: 'Meals' },
    results: 'What there is to add',
    lastAmount: (amount: string) => `last ${amount}`,
    addOf: (name: string) => `Add: ${name}`,
    // A serving's chip, one of it, by its name alone, which may hold a
    // number: "slice (40 g)", "½ cup (100 g)".
    serving: (name: string, grams: string) => `${name} (${grams})`,
    mealLine: (when: string, items: string, kcal: string) => `${when} · ${items} · ${kcal}`,
    repeat: 'Repeat',
    repeatOf: (name: string) => `Repeat: ${name}`,
    // The meals eaten lately at the meal of the day being logged.
    forSlot: {
      breakfast: 'For breakfast',
      lunch: 'For lunch',
      snack: 'For a snack',
      dinner: 'For dinner',
    },
    anEstimateOf: 'An estimate',
    newFood: 'New food',
    back: 'Back to search',
    addToMeal: 'Add to meal',
    // The amount of a food made in the meal, eaten now, in its unit.
    eatenNow: {
      '100g': 'Eaten now (g)',
      '100ml': 'Eaten now (ml)',
      portion: 'Eaten now (portions)',
    },
    basket: 'In the meal',
    empty: 'Pick a food, or repeat a meal.',
    after: (kcal: string) => `after: ${kcal} kcal left`,
    afterOver: (kcal: string) => `after: ${kcal} kcal over`,
    logItems: (n: number, kcal: string) =>
      `Log ${String(n)} ${plural('en', n, { one: 'item', other: 'items' })} · ${kcal} kcal`,
    estimate: 'Add an estimate',
    estimateName: 'What it is',
    amount: (name: string) => `Amount: ${name}`,
    unit: (name: string) => `Unit: ${name}`,
    portions: (n: number) => plural('en', n, { one: 'portion', other: 'portions' }),
    remove: (name: string) => `Remove: ${name}`,
    anEstimate: 'an estimate',
    // Why an amount or an estimate cannot be logged, said at its field:
    // the most in a unit is a function of it and its count.
    wrong: {
      number: 'Write a number, such as 72.5.',
      none: 'The amount must be more than 0.',
      name: 'Give the estimate a name.',
      most: {
        g: (n: string) => `At most ${n} g.`,
        ml: (n: string) => `At most ${n} ml.`,
        serving: (n: string, count: number) =>
          `At most ${n} ${plural('en', count, { one: 'serving', other: 'servings' })}.`,
        portion: (n: string, count: number) =>
          `At most ${n} ${plural('en', count, { one: 'portion', other: 'portions' })}.`,
        kcal: (n: string) => `At most ${n} kcal.`,
      },
    },
    save: 'Log the meal',
    cancel: 'Cancel',
    refused: (detail: string) => `The server did not take this meal: ${detail}`,
  },
  quick: {
    title: 'Quick add',
    // A meal of the day before, by the meal of the day it was.
    yesterday: {
      breakfast: "Yesterday's breakfast",
      lunch: "Yesterday's lunch",
      snack: "Yesterday's snack",
      dinner: "Yesterday's dinner",
    },
  },
  undo: {
    done: (what: string) => `Undone: ${what}.`,
    nothing: 'Nothing to undo.',
    clash: "A later change is in the way, so this can't be undone.",
    notLatest: 'Another change came first, so nothing was undone.',
    failed: (why: string) => `Not undone. ${why}`,
    // What a change taken back did, by its action.
    actions: {
      'food.save': 'a food saved',
      'food.archive': 'a food archived',
      'recipe.save': 'a recipe saved',
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
      'notes.set': 'the notes for your AI coach',
    } as Record<string, string>,
    change: 'a change',
  },
  // The latest changes, made through any client, and the latest's undo.
  changes: {
    title: 'Recent changes',
    // The settings' row of them.
    summary: 'What you and your AI coach changed lately, and its undo',
    none: 'No changes yet.',
    // A meal logged, by its name, as its toast says it but its kcal.
    logged: (what: string) => `Logged: ${what}`,
    aFood: 'a food',
    // A proposal's change, by its status after it.
    proposal: {
      applied: 'A proposal applied',
      rejected: 'A proposal rejected',
      superseded: 'A proposal replaced by a newer one',
      reverted: 'A proposal taken back',
    } as Record<string, string>,
    // The changes said by what they did alone.
    actions: {
      'metrics.merge': 'Measurements imported',
      'profile.set': 'The profile changed',
      'limits.set': 'The limits changed',
      'phase.set': 'A phase set',
      'phase.delete': 'A phase deleted',
      'target.set': 'A target set',
      'target.delete': 'A target deleted',
      'plan.weekdays': 'The training days changed',
      'proposal.create': 'A change of the plan proposed',
    } as Record<string, string>,
    other: 'A change',
    // Who made it: the user in the app, their AI coach, or a device with
    // an access key of its own.
    you: 'you',
    coach: 'your AI coach',
    device: 'a device',
    // When and by whom: "21:14 · you", "Oct 21, 21:14 · you" of a day
    // before.
    by: (when: string, who: string) => `${when} · ${who}`,
    at: (day: string, time: string) => `${day}, ${time}`,
    undone: 'Undone',
    undoOf: (what: string) => `Undo: ${what}`,
  },
  foods: {
    title: 'Foods',
    search: 'Search the foods',
    searchHint: 'A name, a brand or another name',
    lists: 'Lists',
    filter: { inUse: 'In use', archived: 'Archive', estimates: 'Estimates' },
    // The list chosen, and how many foods it has: In use · 142.
    counted: (list: string, n: string) => `${list} · ${n}`,
    // The order's button says the one chosen, in short, and its menu each
    // whole.
    order: (order: string) => `Order: ${order}`,
    orderShort: { frequent: 'Most eaten', use: 'Recent', name: 'By name' },
    orders: { frequent: 'Most eaten first', use: 'Recently eaten first', name: 'By name' },
    // The groups of the foods by how many meals had them lately.
    thisWeek: 'This week',
    earlier: 'Earlier',
    none: 'No foods yet. They appear when your AI coach logs a meal, or add one here.',
    noneArchived: 'No archived foods.',
    noneEstimates: 'No estimates. Every food in use has values from a label or from you.',
    noMatch: 'No food matches.',
    found: 'Found',
    more: 'Show more',
    shown: (shown: number, total: number) => `${String(shown)} of ${String(total)}`,
    add: 'Add a food',
    // The list's button of a new food: short where a language needs it
    // to fit beside the title, whole for a screen reader.
    new: 'New food',
    newSaid: 'New food',
    per: { '100g': 'per 100 g', '100ml': 'per 100 ml', portion: 'per portion' },
    // A food's basis beside its kcal in a list: 370 kcal /100 g.
    perShort: { '100g': '/100 g', '100ml': '/100 ml', portion: '/portion' },
    pick: 'Pick a food to see what it brings, or add one.',
    archivedMark: 'archived',
    // A food's page, read: 370 kcal per 100 g.
    kcalPer: (per: string) => `kcal ${per}`,
    energyFrom: 'Where the energy comes from',
    share: (grams: string, pct: string) => `${grams} g · ${pct}`,
    nutrition: 'Nutrition',
    valuesFor: 'Values for',
    hundred: (unit: string) => `100 ${unit}`,
    onePortion: '1 portion',
    servingOf: (name: string, amount: string) => `${name} (${amount})`,
    row: {
      kcal: 'Energy',
      protein: 'Protein',
      carbs: 'Carbs',
      sugars: 'of which sugars',
      polyols: 'of which polyols',
      fiber: 'Fibre',
      fat: 'Fat',
      saturated_fat: 'of which saturated',
      salt: 'Salt',
      alcohol: 'Alcohol',
    },
    from: 'Values from',
    logged: 'Logged',
    timesLast: (n: string, date: string) => `${n}× · last ${date}`,
    notLogged: 'not yet',
    log: 'Log it',
    logAmount: (amount: string) => `Log ${amount}`,
    edit: 'Edit',
    editTitle: 'Edit the food',
    cancel: 'Cancel',
    energyOff: (macros: string, kcal: string) =>
      `The macros make about ${macros} kcal, not ${kcal}: check the values.`,
    takeKcal: (kcal: string) => `Use ${kcal} kcal`,
    back: 'Back to the foods',
    notFound: 'There is no such food.',
    newTitle: 'New food',
    name: 'Name',
    brand: 'Brand',
    basis: 'The values are',
    nutrient: {
      kcal: 'Energy (kcal)',
      protein: 'Protein (g)',
      carbs: 'Carbs (g)',
      fat: 'Fat (g)',
      fiber: 'Fibre (g)',
      saturated_fat: 'Saturated fat (g)',
      sugars: 'Sugars (g)',
      salt: 'Salt (g)',
      polyols: 'Polyols (g)',
      alcohol: 'Alcohol (g)',
    },
    moreNutrients: 'More nutrients',
    // A food made in a meal being logged: its brand, other nutrients,
    // servings and other names.
    moreFields: 'More',
    portionGrams: 'Grams of a portion',
    servings: 'Servings',
    serving: (n: number) => `Serving ${String(n)}`,
    servingAmount: (unit: string, n: number) => `Amount ${String(n)} (${unit})`,
    servingBounds: 'More than 0, at most 10000.',
    addServing: 'Add a serving',
    removeServing: (name: string) => (name ? `Remove the serving: ${name}` : 'Remove the serving'),
    aliases: 'Other names',
    barcodes: 'Barcodes',
    addBarcode: 'Add a barcode',
    addCode: 'Add the code',
    removeBarcode: (code: string) => `Remove the barcode: ${code}`,
    stopScan: 'Close the camera',
    alias: (n: number) => `Other name ${String(n)}`,
    addAlias: 'Add another name',
    removeAlias: (name: string) => (name ? `Remove the name: ${name}` : 'Remove the name'),
    source: 'Values from',
    sources: {
      label: 'Label',
      estimate: 'Estimate',
      user: 'Me',
      dish: 'Dish',
      off: 'Open Food Facts',
      recipe: 'Recipe',
    },
    note: 'Note',
    save: 'Save the food',
    archive: 'Archive',
    unarchive: 'Bring back',
    archivedNote: 'Archived. Find it among the archived foods; its names stay taken.',
    needed: 'Needed.',
    negative: 'It cannot be less than 0.',
    // A serving's name, or another name, the same as one above it, whatever
    // their case.
    servingTwice: 'Another serving has this name.',
    aliasTwice: 'This other name is given already.',
    // Said at the first serving, or other name, past the most a food has.
    tooManyServings: (most: string, n: number) =>
      `A food has at most ${most} ${plural('en', n, { one: 'serving', other: 'servings' })}.`,
    tooManyAliases: (most: string, n: number) =>
      `A food has at most ${most} other ${plural('en', n, { one: 'name', other: 'names' })}.`,
    kcalPer100: (most: string, unit: string) =>
      `At most ${most} kcal per 100 ${unit}, the energy of pure fat.`,
    massPer100: (most: string, unit: string) =>
      `Protein, carbs, fat and alcohol add up to at most ${most} g per 100 ${unit}.`,
    gramsPer100: (most: string, unit: string) => `At most ${most} g per 100 ${unit}.`,
    kcalPerPortion: (most: string) => `At most ${most} kcal in a portion.`,
    conflict: (detail: string) => `Another food has this name or other name: ${detail}`,
    nameTaken: 'Another food of yours has this name and brand:',
    aliasTaken: 'Another food of yours has this other name:',
    barcodeTaken: 'Another food of yours has one of its barcodes:',
    thatFood: 'that food',
    refused: (detail: string) => `The server did not take this food: ${detail}`,
  },
  // A recipe: a dish cooked again, a food of its own whose values come
  // from its ingredients and the cooked dish's weight.
  recipes: {
    // A recipe's mark beside its name.
    mark: 'recipe',
    list: 'Recipes',
    // The foods' button of something new, a menu of a food or a recipe.
    newChoice: 'New food or recipe',
    newShort: 'New',
    food: 'Food',
    recipe: 'Recipe',
    new: 'New recipe',
    searchHint: 'The name of a recipe',
    none: 'No recipes yet',
    noneWhy:
      'A recipe is a dish you cook again. Add its ingredients once, then log a portion or a share of it.',
    noneCoach: 'Your AI coach can make one too, from a description or a photo.',
    noMatch: 'No recipe matches.',
    // The search of a recipe's ingredients, which finds foods alone.
    nothing: 'No food of yours matches. Make it a new food.',
    noneEaten: 'Nothing eaten yet. Find a food, or make a new one.',
    // A recipe's kcal in a list: 412 kcal /portion.
    perPortionShort: '/portion',
    ingredients: (n: number) =>
      `${String(n)} ${plural('en', n, { one: 'ingredient', other: 'ingredients' })}`,
    portions: (n: string, count: number) =>
      `${n} ${plural('en', count, { one: 'portion', other: 'portions' })}`,
    // A share of the whole dish: ¼ of the recipe.
    ofRecipe: (share: string) => `${share} of the recipe`,
    energy: 'Energy of the recipe',
    kcalPerPortion: 'kcal per portion',
    kcalPer100: 'kcal per 100 g',
    portion: 'Portion',
    per100: 'Per 100 g',
    whole: 'Whole recipe',
    weight: 'Weight',
    weighed: (grams: string) => `Weighed: ${grams}`,
    // Under the weight: 8 portions of 269 g · 85% of the raw ingredients.
    portionsOf: (portions: string, grams: string) => `${portions} of ${grams}`,
    ofRaw: (pct: string) => `${pct} of the raw ingredients`,
    notWeighed: 'Not weighed',
    notWeighedWhy: (raw: string) =>
      `The raw ingredients' sum, ${raw}, stands for the weight. Portions and shares are exact, grams approximate.`,
    noRaw: 'An ingredient has no weight in grams, so weigh the cooked dish.',
    weighIt: 'Enter the weight',
    cookedAgain: 'Cooked again',
    archive: 'Archive the recipe',
    archivedNote: 'Archived. Find it among the archived foods; its name stays taken.',
    ingredientsTitle: 'Ingredients',
    rawSum: "Raw ingredients' sum",
    // An ingredient that is a recipe opens it.
    open: (name: string) => `Open the recipe: ${name}`,
    changedMark: 'values changed',
    changed: (names: string) =>
      `Changed since the recipe was saved: ${names}. The recipe still uses the values it had.`,
    takeNew: 'Take the new values',
    valuesOfSave: "The ingredients' values are those the recipe was last saved with.",
    approx100: 'Per 100 g is approximate: the cooked dish was not weighed.',
    // A save's toast: Saved: Chicken curry · 412 kcal per portion.
    savedPer: (name: string, kcal: string, per: string) => `Saved: ${name} · ${kcal} ${per}`,
    perPortion: 'per portion',
    per100g: 'per 100 g',
    warnings: {
      yield_low: 'The cooked dish weighs far less than its raw ingredients.',
      yield_high: 'The cooked dish weighs far more than its raw ingredients.',
    },
    // The editor.
    newTitle: 'New recipe',
    editTitle: 'Edit the recipe',
    nameHint: 'Chicken curry',
    count: (n: string, most: string) => `${n} / ${most}`,
    noIngredients:
      'No ingredients yet. Add them as you would to a meal. One can be another recipe, such as a sauce.',
    addIngredients: 'Add ingredients',
    mostIngredients: (most: string) => `A recipe has at most ${most} ingredients.`,
    needsIngredient: 'Add at least one ingredient.',
    moveUp: 'Move up',
    moveDown: 'Move down',
    cooked: 'Cooked dish',
    optional: 'optional',
    cookedWeight: 'Weight of the cooked dish',
    fromPot: 'From the pot',
    potHint: 'Weigh the full pot. The empty pot is taken off it.',
    fullPot: 'Full pot',
    emptyPot: 'Empty pot',
    remembered: 'remembered',
    potResult: (grams: string) => `= ${grams} of cooked dish`,
    potWrong: 'The full pot must weigh more than the empty one.',
    shareOfRaw: (pct: string, raw: string) => `${pct} of the raw ingredients' sum (${raw})`,
    lastTime: (weight: string) => `Last time: ${weight}.`,
    lastNotWeighed: 'Last time it was not weighed.',
    weighHint: (raw: string) =>
      `Weigh it if you can, so grams are exact. Otherwise the raw ingredients' sum stands for it: ${raw}.`,
    yieldLow: (grams: string, pct: string, raw: string) =>
      `${grams} is ${pct} of the raw ingredients' sum (${raw}). Was the empty pot taken off? It saves anyway.`,
    yieldHigh: (grams: string, pct: string, raw: string) =>
      `${grams} is ${pct} of the raw ingredients' sum (${raw}). Check the weight. It saves anyway.`,
    cantWeigh: "I can't weigh it",
    cantWeighHint: 'The pot is too big for the scale, say.',
    cantWeighNote: (raw: string) =>
      `The raw ingredients' sum, ${raw}, stands for the weight. Portions and shares are exact, grams approximate.`,
    rawTooHeavy: (most: string) =>
      `The raw ingredients weigh over ${most} g, so weigh the cooked dish.`,
    weightWrong: (most: string) => `More than 0, at most ${most} g.`,
    portionsTitle: 'Portions',
    fewer: 'One portion fewer',
    more: 'One portion more',
    each: (grams: string) => `${grams} each`,
    undivided: 'not divided',
    portionsHint:
      'How many portions the dish makes. Without them, you log a share of the recipe or grams.',
    portionsWrong: (most: string) => `From 1 to ${most} portions.`,
    noteHint: 'Simmer for 40 minutes',
    savesNow: "A save takes the foods' values as they are now. A meal logged never changes.",
    perPortionLabel: 'Per portion',
    per100Label: 'Per 100 g',
    // The editor's foot: Whole recipe 3,296 kcal · ≈ 2,150 g.
    wholeLine: (kcal: string, grams: string) => `Whole recipe ${kcal} · ${grams}`,
    save: 'Save the recipe',
    refused: (detail: string) => `The server did not take this recipe: ${detail}`,
    // The sheet of ingredients.
    into: (name: string) => `Into ${name}`,
    intoNew: 'the new recipe',
    added: 'Added to the recipe',
    addedHint: 'Find a food, type its amount, and add it. This stays open for the next one.',
    done: 'Done',
    doneAdded: (n: number) => `Done · ${String(n)} added`,
    addToRecipe: 'Add to the recipe',
    // The amount of a new food in the recipe, in its unit.
    inRecipe: {
      '100g': 'In the recipe (g)',
      '100ml': 'In the recipe (ml)',
      portion: 'In the recipe (portions)',
    },
    // Cooked again: the same recipe, a new pot.
    cookedHint: 'Same recipe, a new pot. Change what differs. Meals logged stay as they were.',
    was: (what: string) => `was ${what}`,
    otherIngredients: 'An ingredient more or less?',
    saveCooked: 'Save',
    weighTitle: 'Weigh the dish',
    weighSheetHint: 'The same pot, weighed now. With its weight, grams are exact too.',
    saveWeight: 'Save the weight',
    enterWeight: 'Enter the weight, or work it out from the pot.',
    // The servings of a recipe's food, by their names: the whole dish and
    // a portion.
    servings: {
      recipe: (n: number) => plural('en', n, { one: 'recipe', other: 'recipes' }),
      portion: (n: number) => plural('en', n, { one: 'portion', other: 'portions' }),
    },
  },
  charts: {
    title: 'Progress',
    span: 'Period',
    // The periods: short in their segments, whole for a screen reader.
    spans: { '30': '30 d', '90': '90 d', '180': '180 d', '365': '1 y', phase: 'Phase' },
    spansSaid: {
      '30': '30 days',
      '90': '90 days',
      '180': '180 days',
      '365': '1 year',
      phase: 'The phase',
    },
    none: 'Nothing logged in these days yet.',
    noneHint: 'Log a meal or a weigh-in, and how it goes shows here.',
    logMeal: 'Log a meal',
    weight: 'Weight',
    tdee: 'TDEE and intake',
    body: 'Body and recovery',
    sleep: 'Sleep',
    rhr: 'Resting heart rate',
    hrv: 'HRV',
    steps: 'Steps',
    bodyFat: 'Body fat',
    // What a card's number is: the average of the days, the last 7
    // days', or the last value.
    average: 'average',
    lastWeek: 'last 7 days',
    latest: 'latest',
    trend: 'trend',
    // Under the last day's TDEE: half its 80 % interval.
    tdeeNow: (half: string) => `TDEE ± ${half}`,
    // The trend weight's change over the days shown.
    change: (kg: string) => `${kg} kg in these days`,
    // The phase's goal, said when it is far off the weights shown.
    goal: (kg: string) => `Goal ${kg} kg`,
    // The last 7 days' average against the 21 days' before them: +2 vs
    // usual, -7% vs usual.
    vsUsual: (difference: string) => `${difference} vs usual`,
    asUsual: 'as usual',
    // What recovery flags, in a sentence beside the verdict: "Sleep and
    // HRV were worse than usual last week."
    flags: { short_sleep: 'sleep', high_resting_hr: 'resting heart rate', low_hrv: 'HRV' },
    worse: (things: string, count: number) =>
      `${things.charAt(0).toUpperCase()}${things.slice(1)} ${plural('en', count, { one: 'was', other: 'were' })} worse than usual last week.`,
    // The last 7 days' intake against the TDEE, and the weight it moves a
    // week: "You eat about 500 kcal under what you burn: about -0.5 kg a
    // week."
    balance: {
      under: (kcal: string, kg: string) =>
        `You eat about ${kcal} kcal under what you burn, about ${kg} kg a week.`,
      over: (kcal: string, kg: string) =>
        `You eat about ${kcal} kcal over what you burn, about ${kg} kg a week.`,
      even: 'You eat about what you burn: your weight holds.',
    },
    eaten: (kcal: string, days: number) =>
      `Eaten ${kcal} kcal a day, over ${String(days)} logged ${plural('en', days, { one: 'day', other: 'days' })}`,
    // A day in a tooltip: "Mon, Oct 5".
    day: (weekday: string, date: string) => `${weekday}, ${date}`,
    names: {
      weighIn: 'Weigh-in',
      trend: 'Trend',
      goal: 'Goal',
      tdee: 'TDEE',
      interval: '80% interval',
      intake: 'Eaten',
      target: 'Target, 7-day average',
      // The space between the TDEE and the 7 days' intake.
      gap: 'Eaten to TDEE, 7-day average',
      average: '7-day average',
      rhr: 'Resting heart rate',
      hrv: 'HRV',
      steps: 'Steps',
      bodyFat: 'Body fat',
      long: '7 h or more',
      middle: '6 to 7 h',
      short: 'Under 6 h',
    },
    // A day's values in a tooltip, after its date: "85.1 kg · trend 85.4".
    tips: {
      kg: (kg: string) => `${kg} kg`,
      trend: (kg: string) => `trend ${kg}`,
      tdee: (kcal: string, half: string) => `TDEE ${kcal} ± ${half}`,
      eaten: (kcal: string) => `eaten ${kcal}`,
      target: (kcal: string) => `target ${kcal}`,
      average: (hours: string) => `7-day average ${hours}`,
      bpm: (bpm: string) => `${bpm} bpm`,
      ms: (ms: string) => `${ms} ms`,
      steps: (steps: string, count: number) =>
        `${steps} ${plural('en', count, { one: 'step', other: 'steps' })}`,
    },
    // What a chart shows, in words.
    said: {
      weight: (from: string, to: string, weighIns: number) =>
        `Trend weight from ${from} to ${to} kg, ${String(weighIns)} ${plural('en', weighIns, { one: 'weigh-in', other: 'weigh-ins' })}.`,
      // Weigh-ins of today alone: the trend takes them once the day is
      // over.
      weighInsToday: (weighIns: number) =>
        `${String(weighIns)} ${plural('en', weighIns, { one: 'weigh-in', other: 'weigh-ins' })} today; the trend weight takes it once the day is over.`,
      eaten: (kcal: string, days: number) =>
        `${kcal} kcal a day eaten on average, over ${String(days)} logged ${plural('en', days, { one: 'day', other: 'days' })}.`,
      // trust: trusted; before, the estimate of the days before the phase,
      // whose own are too few yet; unsure, not to be trusted yet.
      tdee: (kcal: string, half: string, trust: 'trusted' | 'before' | 'unsure') =>
        `TDEE ${kcal} ± ${half} kcal a day, as of the last day${{ trusted: '', before: ', of the days before the intake last changed', unsure: ', not to be trusted yet' }[trust]}.`,
      sleep: (hours: string) => `${hours} a night on average.`,
      rhr: (bpm: string) => `Resting heart rate ${bpm} bpm on average.`,
      hrv: (ms: string) => `HRV ${ms} ms on average.`,
      bodyFat: (fat: string) => `Body fat ${fat}, as of the last day with it.`,
      steps: (steps: string) => `${steps} steps a day on average.`,
      none: 'No values in these days.',
    },
    // The verdict on the phase, as calibration gives it.
    verdict: {
      label: 'The phase so far',
      onTarget: 'On track.',
      fast: 'Too fast.',
      slow: 'Too slow.',
      waiting: 'Not enough data yet.',
      noPhase: 'No phase yet.',
      noTarget: 'No target yet.',
      losing: (pct: string) => `You're losing ${pct}% a week`,
      gaining: (pct: string) => `You're gaining ${pct}% a week`,
      steady: 'Your weight holds',
      // The phase's range of rates: "0.65–0.85", "−0.85 to −0.65" when the
      // weight goes the other way, "± 0.1" around none.
      range: (lo: string, hi: string) => `${lo}–${hi}`,
      span: (lo: string, hi: string) => `${lo} to ${hi}`,
      around: (pct: string) => `± ${pct}`,
      within: (moving: string, range: string) => `${moving}, within the phase's range (${range}%).`,
      outside: (moving: string, range: string) =>
        `${moving}, outside the phase's range (${range}%).`,
      reasons: {
        no_phase: 'A phase of the plan sets the rate the weight is judged by.',
        no_target: 'The phase is judged once the plan has a target of kcal.',
        before_phase:
          'The phase is new. Until it has enough days of its own, the TDEE comes from the days before it.',
        no_tdee: 'The TDEE came out at 0 or less: are the meals all logged?',
      },
      tdee: (kcal: string, half: string) => `TDEE ${kcal} ± ${half} kcal`,
      trusted: 'Reliable',
      before: 'From before the phase',
      trend: (kg: string) => `Trend weight ${kg} kg`,
      eaten: (kcal: string, target: string) => `Eaten ${kcal} of ${target} kcal a day`,
      setPlan: 'Set up a plan',
      openPlan: 'Open the plan',
    },
    // The table of the weeks.
    weeks: {
      title: 'By week',
      week: 'Week',
      kcal: 'kcal',
      kcalSaid: 'kcal a day',
      protein: 'P',
      proteinSaid: 'Protein, g a day',
      change: 'Δ kg',
      changeSaid: 'Change of the trend weight, kg',
      days: 'Days',
      daysSaid: 'Days logged',
      // Of the week's days that are over: all 7 but this week's.
      daysOf: (n: number, of: number) => `${String(n)}/${String(of)}`,
      none: '—',
      all: (n: number) => `Show all weeks (${String(n)})`,
      fewer: 'Show fewer',
    },
  },
  settings: {
    title: 'Settings',
    appearance: 'Appearance',
    appearanceHint: 'For this browser only, so a phone can be dark and a laptop light.',
    themes: {
      system: 'System',
      light: 'Light',
      dark: 'Dark',
    },
    account: 'Account, language and day',
    accountSummary: (language: string, zone: string, hour: string) =>
      `${language} · ${zone} · the day from ${hour}`,
    email: 'Email',
    name: 'Name',
    nameNeeded: 'A name is needed.',
    language: 'Language',
    timeZone: 'Time zone',
    allZones: 'All time zones',
    noZone: 'No time zone of that name.',
    detectZone: 'Detect automatically',
    dayStart: 'The day starts at',
    dayStartHint: 'A meal before it belongs to the day before.',
    save: 'Save',
    refused: (detail: string) => `The server did not take it: ${detail}`,
    notes: 'Notes for your AI coach',
    notesHint:
      'What your AI coach should know of you: an injury, what you do not eat, how you train.',
    notesPlaceholder: 'Knees: no running. No lactose.',
    noNotes: 'None yet',
    // This device's notifications by Web Push, and every device's.
    notifications: {
      title: 'Notifications',
      hint: 'The morning summary at 10:00, and an alert when your data stop coming.',
      summary: (n: number) =>
        n === 0 ? 'Off' : `${String(n)} ${plural('en', n, { one: 'device', other: 'devices' })}`,
      thisDevice: 'This device',
      on: 'On',
      off: 'Off',
      turnOn: 'Turn on',
      turnOff: 'Turn off',
      unsupported: 'This browser cannot get notifications.',
      install:
        'On an iPhone or iPad, notifications come to the app installed. In Safari, tap Share, then Add to Home Screen, and open tdeemon from there.',
      blocked:
        "Notifications are blocked for tdeemon in this browser's settings. Allow them there, then come back.",
      failed: 'The notifications could not be changed. Try again.',
      devices: 'Devices notified',
      noDevices: 'No device gets them yet.',
      lastSent: (when: string) => `Last notified ${when}`,
      neverSent: 'Nothing sent yet',
      remove: (name: string) => `Stop the notifications to ${name}`,
      removeShort: 'Remove',
      test: 'Send a test',
      tested: (sent: number, failed: number) =>
        failed === 0
          ? `Sent to ${String(sent)} ${plural('en', sent, { one: 'device', other: 'devices' })}.`
          : `Sent to ${String(sent)}, not to ${String(failed)}: its push service did not take it.`,
      testNone: 'No device took it: turn the notifications on first.',
      testFailed: 'The test could not be sent. Try again.',
    },
    devices: 'Devices and AI coaches',
    devicesHint: 'Each device and AI coach signs in with its own access key.',
    devicesSummary: (n: number) =>
      n === 0
        ? 'None yet'
        : `${String(n)} ${plural('en', n, { one: 'access key', other: 'access keys' })}`,
    // The access keys, a card each.
    tokens: 'Connected',
    noTokens: 'Nothing connected yet.',
    noTokensHint: 'Connect your AI coach, or a phone that sends its health data.',
    connect: 'Connect',
    tokenName: 'Name',
    tokenPlaceholder: 'My device',
    scopes: 'What it may do',
    scope: {
      ingest: 'Sends health data',
      read: 'Read only',
      tools: 'Reads and logs',
    },
    create: 'Create the access key',
    created: "Your access key. It's shown only now, so copy it.",
    copy: 'Copy',
    // A button that copies what its label names, among others; and once
    // it has.
    copyOf: (what: string) => `Copy: ${what}`,
    copiedOf: (what: string) => `Copied: ${what}`,
    copied: 'Copied',
    copyFailed: "Couldn't copy. It's selected, so copy it by hand.",
    done: 'Done',
    lastUsed: (when: string) => `Last used ${when}`,
    neverUsed: 'Not used yet',
    revoke: (name: string) => `Revoke: ${name}`,
    confirmRevoke: (name: string) => `Revoke ${name}? It stops working at once.`,
    revokeAll: 'Revoke all access keys',
    confirmRevokeAll:
      'Revoke every access key? Every device and AI coach stops at once, and needs a new one.',
    yesRevoke: 'Revoke',
    keep: 'Keep',
    revoked: (n: number) =>
      `${String(n)} ${plural('en', n, { one: 'access key', other: 'access keys' })} revoked.`,
    signOut: 'Sign out',
    signedInAs: (name: string, email: string) => `Signed in as ${name} (${email}).`,
    signOutFailed: 'Signing out failed. Try again.',
  },
  // Connecting an AI coach or a phone: what it is, its access key, and
  // the steps to give the key to it.
  connect: {
    choose: 'What do you connect?',
    kinds: { coach: 'AI coach', phone: 'iPhone', other: 'Other' },
    kindHints: {
      coach: 'Claude, ChatGPT or another MCP client',
      phone: 'Health Auto Export sends its health data',
      other: 'Advanced: you pick what it may do',
    },
    // The names a new access key starts with.
    names: { coach: 'Claude', phone: 'iPhone' },
    back: 'Back',
    address: "The server's address (MCP)",
    key: 'The access key, shown only now',
    claudeCode: 'Claude Code: run this in a terminal',
    claudeDesktop:
      'Claude Desktop: add this to its claude_desktop_config.json, through mcp-remote, and restart it.',
    chatGpt: "ChatGPT signs in with OAuth, not a key, and tdeemon doesn't offer that yet.",
    first: 'Then write to your AI coach',
    message: 'Look at my data and propose a plan.',
    hae: {
      intro: 'In Health Auto Export, add an automation of the REST API:',
      url: 'URL',
      method: 'Method POST, format JSON',
      header: 'Header Authorization, with this value',
      metrics:
        'Metrics: step count, active energy, basal energy burned, Apple exercise time, sleep analysis, weight, body fat percentage, lean body mass, resting heart rate, heart rate variability and VO2 max',
      summarize: 'Summarize Data: on, with the time grouping by day',
    },
    // An access key of the user's own scopes: how it is sent.
    header: 'It goes in each request, in the header Authorization: Bearer and the key.',
    waiting: 'Waiting for the first call…',
    connected: 'Connected',
    lastCall: (ago: string) => `last call ${ago}`,
  },
  // A new user's first steps to a plan, on today's page.
  firstSteps: {
    title: 'First steps',
    count: (done: number, of: number) => `${String(done)} of ${String(of)}`,
    // Before a step's words, for a screen reader: whether it is done.
    done: 'Done:',
    toDo: 'To do:',
    profile: 'Complete your profile',
    profileDone: 'Your profile',
    weighIn: 'First weigh-in',
    connect: 'Connect your AI coach or a phone',
    connectDone: 'Your AI coach or a phone',
    meal: 'Log your first meal',
    mealDone: 'Your first meal',
    ask: 'Ask your AI coach for a plan',
    write: 'Write this to your AI coach',
    later: 'Later',
    // The plan's way to show them again, once put off.
    show: 'Show the first steps',
  },
  plan: {
    title: 'Plan',
    you: 'You and limits',
    profile: 'Profile',
    profileHint: "A plan needs all of these; logging meals doesn't. Clear a field to remove it.",
    sex: 'Sex',
    sexHint: 'Used to estimate how much energy you burn.',
    sexes: { male: 'Male', female: 'Female' },
    noSex: 'Not given',
    birthDate: 'Birth date',
    height: 'Height',
    missing: (what: string) => `A plan still needs: ${what}.`,
    complete: 'A plan has all it needs of you.',
    missingField: { sex: 'sex', birth_date: 'birth date', height_cm: 'height' },
    wrongDate: 'The birth date is not a whole date: give it all, or clear it.',
    dateToCome: 'The birth date is after today.',
    tooOld: (age: string) => `The birth date makes an age of more than ${age}.`,
    wrongHeight: (min: string, max: string) => `The height is ${min} to ${max} cm.`,
    age: (n: number) => `${String(n)} ${plural('en', n, { one: 'year', other: 'years' })}`,
    save: 'Save',
    limits: 'Limits of the plan',
    limitsHint:
      "They keep a plan within what is safe for a healthy adult, and are no medical advice. Tighten them freely; loosening one asks you first, and goes no further than the instance's.",
    limitsSummary: (loss: string, protein: string) =>
      `Loss up to ${loss} · protein from ${protein}`,
    groups: { rates: 'Pace', food: 'Food', autopilot: 'Autopilot' },
    groupHints: {
      rates: 'How fast a phase may change your weight.',
      food: 'The least and the most a day of the plan may hold.',
      autopilot: 'How far and how often the autopilot may move your targets.',
    },
    changed: (n: number) => (n === 0 ? 'As by default' : `${String(n)} changed`),
    toFix: (n: number) => `${String(n)} to fix`,
    limit: {
      cut_rate: 'Fastest loss in a cut, BMI under 30',
      cut_rate_obese: 'Fastest loss in a cut, BMI 30 or more',
      loss_kg_week: 'Most weight lost in a week',
      bulk_rate: 'Fastest gain in a bulk',
      recomp_min: 'Lowest rate in a recomp',
      recomp_max: 'Highest rate in a recomp',
      protein_floor: 'Least protein',
      protein_ceiling: 'Most protein',
      protein_floor_grams: 'Least protein in grams (0 for none)',
      kcal_floor_male: 'Fewest kcal of a day, for a man',
      kcal_floor_female: 'Fewest kcal of a day, for a woman',
      fat_floor_pct: 'Least fat',
      step_kcal: 'Largest step of the autopilot',
      step_pct: 'Largest step, of the weekly average',
      interval_days: 'Days between two steps, at least',
      first_step_days: 'Days before the first step of a phase',
      logged_days: 'Logged days a step needs',
      weigh_ins: 'Weigh-ins a step needs',
      drift_pct: 'Most drift from the target you agreed to',
    },
    unit: {
      pctWeek: '% a week',
      kgWeek: 'kg a week',
      gPerKg: 'g/kg',
      g: 'g',
      kcal: 'kcal',
      pctKcal: '% of kcal',
      pctAvg: '%',
      pctTdee: '% of TDEE',
      days: (n: number) => plural('en', n, { one: 'day', other: 'days' }),
      ofWeek: 'of 7 days',
    },
    defaults: (value: string) => `Default ${value}`,
    instance: (value: string) => `instance limit ${value}`,
    holds: (value: string) => `Holds: ${value}, the instance's`,
    problem: {
      number: 'Not a number.',
      whole: 'A whole number of days.',
      range: (min: string, max: string) => `From ${min} to ${max}.`,
      instance: (value: string) => `Past the instance's limit, ${value}.`,
      order: (other: string) => `Past ${other}.`,
      lower: (other: string, value: string) =>
        `Past the instance's ${other}, ${value}: lower it to ${value} at most, or no change of your limits is taken.`,
      raise: (other: string, value: string) =>
        `Below the instance's ${other}, ${value}: raise it to ${value} at least, or no change of your limits is taken.`,
    },
    loosen: (names: string) => `Loosening ${names} allows a plan the limit refused. Loosen it?`,
    yesLoosen: 'Loosen',
    keep: 'Back',
    unchanged: 'Nothing to save.',
    refused: (detail: string) => `The server did not take it: ${detail}`,
    phase: {
      current: (since: string) => `Current phase · since ${since}`,
      none: 'No phase now',
      next: (type: string, date: string) => `Next: ${type}, from ${date}`,
      week: (n: number) => `Week ${String(n)}`,
      weekOf: (n: number, of: number) => `Week ${String(n)} of ${String(of)}`,
      pace: 'Target pace',
      kg: (kg: string) => `≈ ${kg} kg a week`,
      band: (from: string, to: string) => `in range from ${from} to ${to}`,
      measured: 'Measured',
      notMeasured: 'Not measured yet',
      rates: { none: '', on_target: 'in range', below: 'below range', above: 'above range' },
      progress: 'Days of the phase',
      left: (n: number) =>
        n === 0
          ? 'Its last day'
          : `${String(n)} ${plural('en', n, { one: 'day', other: 'days' })} left`,
      span: (from: string, to: string) => `${from} – ${to}`,
      from: (from: string) => `from ${from}`,
      goal: (kg: string) => `Goal ${kg} kg`,
      milestone: (kg: string) => `milestone ${kg} kg`,
      // A milestone with no goal after it.
      milestoneAlone: (kg: string) => `Milestone ${kg} kg`,
      trend: (kg: string, toGo: string) => `trend ${kg} kg, ${toGo} kg to go`,
      timeline: 'Phases',
      states: { past: 'Past', now: 'Now', planned: 'Planned' },
    },
    targets: {
      title: 'Targets by type of day',
      since: (date: string) => `since ${date}`,
      from: (date: string) => `From ${date}`,
      training: 'Training',
      rest: 'Rest',
      trainingDays: 'Training days',
      days: (n: number) => `${String(n)} ${plural('en', n, { one: 'day', other: 'days' })} a week`,
      everyDay: 'every day',
      noDay: 'no day',
      none: 'No target yet.',
      average: 'Weekly average',
      perDay: (kcal: string) => `${kcal} kcal a day`,
      tdee: 'TDEE',
      noTdee: 'not measured yet',
      // The weekly average against the TDEE, said in words, so the number
      // beside them is never below 0.
      deficit: 'Planned deficit',
      surplus: 'Planned surplus',
      even: 'Planned balance',
      balance: (kcal: string) => `${kcal} kcal a day, weekly average`,
    },
    empty: {
      title: 'No plan yet',
      text: 'Your AI coach proposes one: ask it to "look at the last month and propose a plan". Its proposal waits here for your answer.',
      profile: 'Complete your profile',
      coach: 'Connect an AI coach',
    },
    pending: {
      title: 'A proposed change',
      by: (who: string) => `Proposed by ${who}`,
      // A proposal of the user's own that waits: a revision taken back
      // that the limits refused then.
      byYou: 'Your own proposal',
      // Who proposed it: the model's maker, or the AI coach, and the
      // model.
      who: (name: string, model: string) => `${name} (${model})`,
      coach: 'your AI coach',
      window: (from: string, to: string) => `Drawn from the days ${from} to ${to}.`,
      expiresIn: (days: number) =>
        `expires in ${String(days)} ${plural('en', days, { one: 'day', other: 'days' })}`,
      data: {
        title: 'What the data say',
        rate: {
          on_target: (pct: string, from: string, to: string) =>
            `Calibration measures ${pct} a week, within the phase's range (${from} to ${to}).`,
          below: (pct: string, from: string, to: string) =>
            `Calibration measures ${pct} a week, below the phase's range (${from} to ${to}).`,
          above: (pct: string, from: string, to: string) =>
            `Calibration measures ${pct} a week, above the phase's range (${from} to ${to}).`,
        },
        noVerdict: 'Calibration has no verdict on the phase yet.',
        trusted: (kcal: string, pm: string) => `Its TDEE, ${kcal} ± ${pm} kcal, is reliable.`,
        notTrusted: (kcal: string) => `Its TDEE, about ${kcal} kcal, is not reliable yet.`,
        noTdee: 'No TDEE measured yet.',
        inRange: "The pace is in range, so the data alone don't ask for this change.",
        more: 'The data point the other way: they ask for more kcal, not fewer.',
        fewer: 'The data point the other way: they ask for fewer kcal, not more.',
      },
      accept: 'Accept',
      reject: 'Reject',
      sending: 'Sending…',
      gone: 'This proposal is no longer open. The plan changed, or it expired.',
      refusedNow: 'The limits refuse it now:',
      failed: 'It did not go through. Try again.',
      stale: "It can't be applied anymore, as it would now start before today. Ask for a new one.",
      limitsHere:
        'A change of your limits is accepted only here, in the app, never by your AI coach.',
      average: (from: string, kcal: string) => `From ${from}: ${kcal} kcal a day on average`,
      rate: (pct: string, kg: string) => `, ${pct} a week (${kg} kg)`,
    },
    change: {
      newPhase: (from: string) => `New phase from ${from}`,
      phase: (from: string) => `Phase from ${from}`,
      deletedPhase: (from: string) => `Phase from ${from}, deleted`,
      newTarget: (from: string) => `New target from ${from}`,
      target: (from: string) => `Target from ${from}`,
      deletedTarget: (from: string) => `Target from ${from}, deleted`,
      weekdays: 'Training days',
      noDays: 'none',
      types: { cut: 'Cut', maintain: 'Maintenance', bulk: 'Bulk', recomp: 'Recomp' },
      rate: (pct: string) => `${pct} a week`,
      until: (date: string) => `until ${date}`,
      goal: (kg: string) => `goal ${kg} kg`,
      milestone: (kg: string) => `milestone ${kg} kg`,
      training: 'Training day',
      rest: 'Rest day',
      day: (kcal: string, protein: string, carbs: string, fat: string) =>
        `${kcal} kcal · P ${protein} g · C ${carbs} g · F ${fat} g`,
      was: (what: string) => `was ${what}`,
      limit: (name: string) => `Limit: ${name}`,
      loosened: 'loosened',
      tightened: 'tightened',
      // A change of a value, from what it was to what it is to be.
      to: (from: string, to: string) => `${from} → ${to}`,
      // The days a change of a target is of, as a line starts or within
      // one.
      days: { training: 'training days', rest: 'rest days', every: 'every day' },
      kcal: (from: string, to: string) => `${from} → ${to} kcal`,
      grams: (name: string, from: string, to: string) => `${name} ${from} → ${to} g`,
      // A phase with no planned end, until the next one.
      open: 'no end',
    },
    // The rationales of the changes made in the app, by their codes, which
    // the server stores (proposal.ChangedInTheApp).
    rationales: {
      'changed in the app': 'Changed in the app.',
      'taken back in the app': 'Taken back in the app.',
    },
    // The user, as who changed the plan.
    byUser: 'you',
    // The line on the day that a proposal waits.
    nudge: {
      coach: (what: string) => `Your AI coach suggests: ${what}`,
      yours: (what: string) => `A change waits for your answer: ${what}`,
      review: 'Review',
    },
    edit: {
      phase: 'Edit phase',
      next: 'Next phase',
      targets: 'Edit',
      targetsLabel: 'Edit targets',
      targetsTitle: 'Targets by type of day',
      type: 'Type',
      start: 'Starts on',
      end: 'Ends on',
      endHint: 'Empty: until the next phase starts.',
      since: (type: string, date: string) => `${type}, since ${date}`,
      loss: 'Loss (% a week)',
      gain: 'Gain (% a week)',
      rate: 'Rate (% a week, a minus is a loss)',
      goal: 'Goal weight, kg',
      milestone: 'Milestone weight, kg',
      kcal: 'kcal',
      protein: 'Protein, g',
      fat: 'Fat, g',
      carbs: (g: string) => `Carbs follow: ${g} g`,
      fromToday: 'From today on, until a change.',
      save: 'Save',
      saving: 'Saving…',
      problem: {
        number: 'Not a number.',
        positive: 'More than 0.',
        most: (n: string) => `At most ${n}.`,
        range: (min: string, max: string) => `From ${min} to ${max}.`,
        over: 'Protein and fat give more kcal than the day has.',
        endBefore: 'Before the phase starts.',
        endPast: 'Before today. Past days stay as they were.',
        startAfter: (date: string) => `After ${date}.`,
        startPast: 'Today or later.',
      },
      atLeast: (value: string) => `At least ${value}`,
      atMost: (value: string) => `At most ${value}`,
      levels: {
        user: 'your limit',
        instance: "the instance's limit",
        code: 'a limit nothing changes',
      },
      refused: 'The limits do not allow it.',
      loosen: 'Loosen it in your limits',
      saved: { phase: 'Phase changed', next: 'Next phase planned', targets: 'Targets changed' },
    },
    revisions: {
      title: 'Plan history',
      none: 'No change of the plan yet.',
      applied: (date: string, who: string) => `${date}, by ${who}`,
      reverted: 'Taken back',
      revert: 'Take back',
      confirm: 'Take this change back, from now on?',
      loosens: (names: string) =>
        `Taking it back loosens ${names}, which allows a plan the limit refused.`,
      yes: 'Take back',
      no: 'Keep it',
      cannot:
        "This change can't be taken back anymore. It took effect, or the plan changed since. Ask for a new change.",
    },
    rules: {
      profile_missing: 'a plan needs your profile',
      weight_missing: 'a plan needs a weigh-in',
      min_age: 'no plan under 18',
      underweight_loss: 'no loss below a BMI of 18.5',
      underweight_goal: 'a goal below a BMI of 18.5',
      underweight_milestone: 'a milestone below a BMI of 18.5',
      cut_rate: 'the loss of the cut',
      bulk_rate: 'the gain of the bulk',
      recomp_rate: 'the rate of the recomp',
      loss_kg_week: 'the loss a week',
      implied_loss_rate: 'the loss the target makes',
      implied_gain_rate: 'the gain the target makes',
      implied_loss_kg_week: 'the loss a week the target makes',
      kcal_floor: 'the kcal of a day',
      protein_floor: 'the protein',
      protein_ceiling: 'the protein',
      protein_floor_grams: 'the protein in grams',
      fat_floor: 'the fat',
      invalid_change: 'the change is not whole',
    },
    levels: {
      user: 'your limit, which you may loosen below',
      instance: "the instance's limit",
      code: 'a limit nothing changes',
    },
    proposedLimit: (proposed: string, limit: string, level: string) =>
      `${proposed} proposed, the limit ${limit}: ${level}`,
    // What a warning of the limits means, in plain words: the value, what
    // is advised, and why it matters.
    warnings: {
      below_bmr: (value: string, threshold: string) =>
        `Below your resting burn (${value} kcal, BMR about ${threshold}). Fine for a while, but hard to keep up.`,
      low_energy_availability: (value: string, threshold: string) =>
        `Low energy for your lean mass (${value} kcal/kg, at least ${threshold} advised). Over time it can tire you and slow recovery.`,
      low_carbs: (value: string, threshold: string) =>
        `Few carbs (${value} g, at least ${threshold} g advised). Fine if you choose it, but training may feel harder.`,
    },
  },
  notFound: {
    title: 'Page not found',
    back: 'Back to the start',
  },
}

export type Messages = typeof en
