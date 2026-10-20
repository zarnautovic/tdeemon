import { Link } from '@tanstack/react-router'

import { buttonVariants } from '@/components/ui/button'

export function NotFoundPage() {
  return (
    <section className="space-y-4">
      <h1 className="text-2xl font-semibold">Page not found</h1>
      <Link to="/" className={buttonVariants({ variant: 'outline' })}>
        Back to the start
      </Link>
    </section>
  )
}
