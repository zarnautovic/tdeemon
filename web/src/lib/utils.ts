import { createCn } from 'cn/config'

// cn joins class names, the later winning a conflict, as tailwind-merge
// does. It is told the theme's own text sizes and shadows (index.css):
// unknown, text-body would pass for a colour, and text-destructive after
// it would drop the size. The shadcn components import it from here
// (components.json).
export const cn = createCn({
  extend: {
    classGroups: {
      'font-size': [{ text: ['display', 'title', 'h2', 'h3', 'body', 'small', 'caption'] }],
      shadow: [{ shadow: ['card', 'sheet'] }],
    },
  },
})
