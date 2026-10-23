import { Link } from '@tanstack/react-router'

import { buttonVariants } from '@/components/ui/button'
import { useMessages } from '@/i18n/messages'

export function NotFoundPage() {
  const t = useMessages()

  return (
    <section className="space-y-4">
      <h1 className="text-2xl font-semibold">{t.notFound.title}</h1>
      <Link to="/" className={buttonVariants({ variant: 'outline' })}>
        {t.notFound.back}
      </Link>
    </section>
  )
}
