import { en, type Messages } from './en'
import { hr } from './hr'
import { useLocale, type Locale } from './locale'

// Every language the app speaks has its messages: a Locale without them
// fails the type check.
const dictionaries: Record<Locale, Messages> = { en, hr }

// Reason is the code of a reason for leaving a day out that a tap gives.
type Reason = keyof Messages['day']['reasons']

// isReason tells a code of a reason from another string, as Object.keys
// gives the codes.
function isReason(code: string): code is Reason {
  return code in en.day.reasons
}

// reasonOf is the code of reason, if it is one a tap gives: its code, or
// what the tap says in any language, as a day left out before the taps
// gave codes holds it. Undefined for a reason typed.
export function reasonOf(reason: string): Reason | undefined {
  const typed = reason.trim()
  return Object.keys(en.day.reasons)
    .filter(isReason)
    .find(
      (code) =>
        code === typed || Object.values(dictionaries).some((d) => d.day.reasons[code] === typed),
    )
}

// useMessages returns the app's text in the current language, e.g.
// useMessages().nav.day; a component that uses it renders again when the
// language changes.
export function useMessages(): Messages {
  return dictionaries[useLocale()]
}
