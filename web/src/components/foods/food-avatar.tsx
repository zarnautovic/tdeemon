import { cn } from '@/lib/utils'

const graphemes = new Intl.Segmenter()

// first is a word's first letter as a reader sees it: Č, or an emoji,
// whole.
function first(word: string): string {
  for (const { segment } of graphemes.segment(word)) {
    return segment
  }
  return ''
}

// initials are a name's first letters, of its first two words.
function initials(name: string): string {
  return name
    .split(/\s+/)
    .filter((w) => w !== '')
    .slice(0, 2)
    .map(first)
    .join('')
    .toUpperCase()
}

// FoodAvatar is a food's mark beside its name: the initials of its name,
// for the eye, as the name beside it says the same. large is a food's
// page's.
export function FoodAvatar({ name, large = false }: { name: string; large?: boolean }) {
  return (
    <span
      aria-hidden
      className={cn(
        'grid shrink-0 place-items-center rounded-md bg-primary-soft font-semibold text-primary',
        large ? 'size-14 text-h3' : 'size-10 text-small',
      )}
    >
      {initials(name)}
    </span>
  )
}
