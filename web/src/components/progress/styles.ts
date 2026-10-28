// The classes the progress's cards share.

// card is a card of the progress: 16 px in on a phone, 20 on a desktop.
export const card = 'rounded-lg border bg-card p-4 lg:p-5'

// entry is a card coming in, once it has its data: it fades in from a
// little below in 200 ms (with less motion asked for, it only fades).
export const entry =
  'transition-[opacity,translate] duration-(--duration-base) ease-out starting:translate-y-1 starting:opacity-0'
