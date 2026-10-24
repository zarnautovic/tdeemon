import table from './name-key-table.json'

// The table is the server's case folding (golang.org/x/text/cases.Fold)
// of every rune it changes, and the runes its strings.Fields splits at,
// written from food.Key by go test ./internal/domain/food -run TestWebKey
// -update: a range folds first, first+step, … (count of them) each by
// delta; a special to its string.
const folds = new Map<number, string>()
for (const [first = 0, count = 0, step = 1, delta = 0] of table.ranges) {
  for (let i = 0; i < count; i++) {
    const r = first + i * step
    folds.set(r, String.fromCodePoint(r + delta))
  }
}
for (const [r, to] of Object.entries(table.specials)) {
  folds.set(Number(r), to)
}
const spaces = new Set(table.spaces)

// nameKey is a name as the server tells names apart (food.Key): in NFC,
// case folded, its words one space apart.
export function nameKey(name: string): string {
  const words: string[] = []
  let word = ''
  for (const c of name.normalize('NFC')) {
    const r = c.codePointAt(0) ?? 0
    if (spaces.has(r)) {
      if (word !== '') {
        words.push(word)
        word = ''
      }
      continue
    }
    word += folds.get(r) ?? c
  }
  if (word !== '') {
    words.push(word)
  }
  return words.join(' ')
}
