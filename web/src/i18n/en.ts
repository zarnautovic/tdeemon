// The app's text in English, the base language. Every other language has
// the same keys (Messages), which the type checker holds it to; a text
// with values in it is a function of them.
export const en = {
  nav: {
    label: 'Sections',
    day: 'Day',
    foods: 'Foods',
    charts: 'Charts',
    settings: 'Settings',
  },
  soon: 'This part is on its way.',
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
    about: 'About',
    connecting: 'Connecting to the server…',
    unreachable: 'The server cannot be reached.',
  },
  notFound: {
    title: 'Page not found',
    back: 'Back to the start',
  },
}

export type Messages = typeof en
