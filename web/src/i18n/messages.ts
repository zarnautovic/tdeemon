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

// Rationale is the code of the rationale of a change made in the app.
type Rationale = keyof Messages['plan']['rationales']

function isRationale(code: string): code is Rationale {
  return code in en.plan.rationales
}

// rationaleOf is the code of a proposal's rationale, if the app made it:
// its code, or what the app says of it in any language, as a revision
// taken back before the app sent codes holds it. Undefined for a
// rationale a coach wrote.
export function rationaleOf(rationale: string): Rationale | undefined {
  const written = rationale.trim()
  return Object.keys(en.plan.rationales)
    .filter(isRationale)
    .find(
      (code) =>
        code === written ||
        Object.values(dictionaries).some((d) => d.plan.rationales[code] === written),
    )
}

// useMessages returns the app's text in the current language, e.g.
// useMessages().nav.today; a component that uses it renders again when
// the language changes.
export function useMessages(): Messages {
  return dictionaries[useLocale()]
}

// messagesOf is the app's text in a language, before the app speaks it:
// what is said of a change of the language, in the language chosen.
export function messagesOf(locale: Locale): Messages {
  return dictionaries[locale]
}
