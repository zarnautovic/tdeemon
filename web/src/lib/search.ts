// fold is s as a search of foods tells it apart: in lower case, without
// diacritics. A đ stays, as no d and mark make it up.
function fold(s: string): string {
  return s
    .normalize('NFD')
    .replaceAll(/\p{Mn}/gu, '')
    .toLowerCase()
}

// split is the words of a text folded: its letters and numbers.
function split(s: string): string[] {
  return s.split(/[^\p{L}\p{N}]+/u).filter((w) => w !== '')
}

// forms are the forms of a word folded that a search finds it by, as the
// server's find_foods indexes a name (searchText): with a đ in it, the
// word with d and with dj in its place, as Croatian is typed both ways
// without the stroke: "grožđe" is found as "grozde" and as "grozdje".
function forms(word: string): string[] {
  return word.includes('đ') ? [word.replaceAll('đ', 'd'), word.replaceAll('đ', 'dj')] : [word]
}

// sought are the words of a query, a đ in them as dj, as find_foods looks
// for them.
function sought(query: string): string[] {
  return split(fold(query).replaceAll('đ', 'dj'))
}

// matches tells whether each word of query starts a word of one of
// texts, as find_foods finds a food by the start of the words of its
// name, brand or an alias: in any case, without diacritics, a đ typed as
// d or dj. A query of no words matches anything.
export function matches(query: string, texts: readonly (string | undefined)[]): boolean {
  const have = texts.flatMap((t) => (t === undefined ? [] : split(fold(t)).flatMap(forms)))
  return sought(query).every((w) => have.some((h) => h.startsWith(w)))
}

// A part of a text, from at, and whether a query matched it.
export interface Part {
  at: number
  text: string
  matched: boolean
}

// The words of a text as it is written: letters and numbers, with the
// marks fold takes off them.
const written = /[\p{L}\p{N}\p{Mn}]+/gu

// parts are text in the parts a query matched and those it did not, as
// matches tells them: of each word of text, as much of its start as the
// longest word of query that starts it covers.
export function parts(query: string, text: string): Part[] {
  const want = sought(query)
  const out: Part[] = []
  let at = 0
  for (const word of text.matchAll(written)) {
    const end = Math.max(0, ...want.map((w) => reach(word[0], w)))
    if (end === 0) {
      continue
    }
    if (word.index > at) {
      out.push({ at, text: text.slice(at, word.index), matched: false })
    }
    out.push({ at: word.index, text: text.slice(word.index, word.index + end), matched: true })
    at = word.index + end
  }
  if (at < text.length) {
    out.push({ at, text: text.slice(at), matched: false })
  }
  return out
}

// reach is how much of word, from its start, w covers: the shortest
// start of it whose forms start with w, with the marks on its last
// letter; 0 for a word w does not start.
function reach(word: string, w: string): number {
  const starts = (s: string) => forms(fold(s)).some((f) => f.startsWith(w))
  if (!starts(word)) {
    return 0
  }
  let end = 1
  while (end < word.length && !starts(word.slice(0, end))) {
    end++
  }
  while (end < word.length && /\p{Mn}/u.test(word.charAt(end))) {
    end++
  }
  return end
}
