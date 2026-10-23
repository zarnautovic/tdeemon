import { en, type Messages } from './en'
import { hr } from './hr'
import { useLocale, type Locale } from './locale'

// Every language the app speaks has its messages: a Locale without them
// fails the type check.
const dictionaries: Record<Locale, Messages> = { en, hr }

// useMessages returns the app's text in the current language, e.g.
// useMessages().nav.day; a component that uses it renders again when the
// language changes.
export function useMessages(): Messages {
  return dictionaries[useLocale()]
}
