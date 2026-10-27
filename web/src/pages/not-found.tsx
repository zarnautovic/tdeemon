import { Link } from '@tanstack/react-router'

import { buttonVariants } from '@/components/ui/button'
import { useMessages } from '@/i18n/messages'
import { narrowPage } from '@/lib/styles'

export function NotFoundPage() {
  const t = useMessages()

  return (
    <main className={narrowPage}>
      <h1 className="text-2xl font-semibold">{t.notFound.title}</h1>
      <Link to="/" className={buttonVariants({ variant: 'outline' })}>
        {t.notFound.back}
      </Link>
    </main>
  )
}
