import type { Locale } from './locale'

// PluralForms are a word's forms by count, as CLDR's plural rules name
// them (hr has one, few and other: 1 minuta, 2 minute, 5 minuta); other
// is the one every language has.
export type PluralForms = Partial<Record<Intl.LDMLPluralRule, string>> & { other: string }

export function plural(locale: Locale, n: number, forms: PluralForms): string {
  return forms[new Intl.PluralRules(locale).select(n)] ?? forms.other
}
