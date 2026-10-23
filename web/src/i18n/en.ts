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
  day: {
    title: 'Today',
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
    connecting: 'Connecting to the server…',
  },
  notFound: {
    title: 'Page not found',
    back: 'Back to the start',
  },
}

export type Messages = typeof en
