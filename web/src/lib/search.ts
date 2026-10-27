// words are the words of s as a search of foods tells them apart: its
// letters and numbers, in lower case, without diacritics, a đ as dj, as
// the server's find_foods has them.
function words(s: string): string[] {
  return s
    .normalize('NFD')
    .replaceAll(/\p{Mn}/gu, '')
    .toLowerCase()
    .replaceAll('đ', 'dj')
    .split(/[^\p{L}\p{N}]+/u)
    .filter((w) => w !== '')
}

// matches tells whether each word of query starts a word of one of
// texts, as find_foods finds a food by the start of the words of its
// name, brand or an alias: in any case, without diacritics. A query of no
// words matches anything.
export function matches(query: string, texts: readonly (string | undefined)[]): boolean {
  const have = texts.flatMap((t) => (t === undefined ? [] : words(t)))
  return words(query).every((w) => have.some((h) => h.startsWith(w)))
}
