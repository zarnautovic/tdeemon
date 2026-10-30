import limits from '@/lib/limits.json'

// A Weighing is the cooked dish's weight as typed: the grams, or the full
// pot and the empty one they come from (pot, open or not), or that it
// cannot be weighed.
export interface Weighing {
  weight: string
  pot: boolean
  full: string
  empty: string
  cantWeigh: boolean
}

// weighingOf is a weighing to start with: the dish's weight as typed
// before, if any, and the empty pot remembered (potG), whose fields are
// open when there is one.
export function weighingOf(
  field: (n: number) => string,
  potG: number | undefined,
  weight?: number,
): Weighing {
  return {
    weight: weight === undefined ? '' : field(weight),
    pot: potG !== undefined,
    full: '',
    empty: potG === undefined ? '' : field(potG),
    cantWeigh: false,
  }
}

// withPot is a weighing whose pot changed: its weight, the full pot less
// the empty one, once the full pot is the heavier.
export function withPot(
  w: Weighing,
  change: Partial<Pick<Weighing, 'full' | 'empty'>>,
  parse: (s: string) => number | undefined,
  field: (n: number) => string,
): Weighing {
  const next = { ...w, ...change }
  const full = parse(next.full)
  const empty = parse(next.empty)
  if (full !== undefined && empty !== undefined && empty >= 0 && full > empty) {
    return { ...next, weight: field(full - empty) }
  }
  return next
}

// yieldOfWeighing is the cooked dish's weight a weighing gives, or
// undefined when it is not weighed (or not a number yet).
export function yieldOfWeighing(
  w: Weighing,
  parse: (s: string) => number | undefined,
): number | undefined {
  if (w.cantWeigh) {
    return undefined
  }
  const n = parse(w.weight)
  return n !== undefined && n > 0 ? n : undefined
}

// potOf is the empty pot a weighing keeps for the next one: the one
// typed, else the one remembered.
export function potOf(
  w: Weighing,
  parse: (s: string) => number | undefined,
  remembered: number | undefined,
): number | undefined {
  const n = parse(w.empty)
  return n !== undefined && n > 0 && n <= limits.recipe.maxWeight ? n : remembered
}

// WeighingWrong is what is wrong with a weighing: its weight is not a
// number, or is out of bounds (weight), or the full pot is not heavier
// than the empty one (pot).
export interface WeighingWrong {
  weight?: true
  pot?: true
}

export function weighingWrong(
  w: Weighing,
  parse: (s: string) => number | undefined,
): WeighingWrong {
  if (w.cantWeigh) {
    return {}
  }
  const wrong: WeighingWrong = {}
  if (w.weight.trim() !== '') {
    const n = parse(w.weight)
    if (n === undefined || n <= 0 || n > limits.recipe.maxWeight) {
      wrong.weight = true
    }
  }
  const full = parse(w.full)
  const empty = parse(w.empty)
  if (w.pot && full !== undefined && empty !== undefined && !(full > empty)) {
    wrong.pot = true
  }
  return wrong
}
