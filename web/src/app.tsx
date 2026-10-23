import { QueryClientProvider, type QueryClient } from '@tanstack/react-query'
import { RouterProvider } from '@tanstack/react-router'
import { useEffect } from 'react'

import { useLocale } from '@/i18n/locale'
import type { AppRouter } from '@/router'

export function App({ router, queryClient }: { router: AppRouter; queryClient: QueryClient }) {
  const locale = useLocale()
  // The document's language follows the app's, for screen readers,
  // hyphenation and the browser's own controls.
  useEffect(() => {
    document.documentElement.lang = locale
  }, [locale])

  return (
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
}
