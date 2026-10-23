import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useNavigate, useSearch } from '@tanstack/react-router'
import type { SubmitEvent } from 'react'

import { $api } from '@/api/client'
import { HttpError } from '@/api/errors'
import { safeTarget, sessionQuery } from '@/api/session'
import { signIn } from '@/api/sign-in'
import { Button, buttonVariants } from '@/components/ui/button'
import type { Messages } from '@/i18n/en'
import { useMessages } from '@/i18n/messages'

const inputClass =
  'h-10 w-full rounded-lg border border-input bg-background px-3 outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50'

// text is a text field's value in a form.
function text(form: FormData, name: string): string {
  const value = form.get(name)
  return typeof value === 'string' ? value : ''
}

// problem is what to tell a person whose sign-in failed.
function problem(t: Messages, error: unknown): string {
  if (error instanceof HttpError) {
    if (error.status === 401) {
      return t.signIn.wrong
    }
    if (error.status === 429 && error.retryAfter !== undefined) {
      return t.signIn.tooMany(error.retryAfter)
    }
    if (error.status === 503 && error.retryAfter !== undefined) {
      return t.signIn.busy(error.retryAfter)
    }
    return t.signIn.failed
  }
  return t.unreachable
}

// LoginPage signs a person in with a password, or through the instance's
// OpenID Connect provider, and then goes where they were headed.
export function LoginPage() {
  const t = useMessages()
  const target = safeTarget(useSearch({ from: '/login' }).redirect)
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const methods = $api.useQuery('get', '/api/v1/auth/methods')
  const login = useMutation({
    mutationFn: signIn,
    onSuccess: async (session) => {
      // Nothing another user left in the app (a session that ended
      // rather than signed out) stays for this one; the sections take
      // the user's language from the session.
      queryClient.clear()
      queryClient.setQueryData(sessionQuery.queryKey, session)
      await navigate({ href: target, replace: true })
    },
  })

  function submit(event: SubmitEvent<HTMLFormElement>) {
    event.preventDefault()
    const form = new FormData(event.currentTarget)
    login.mutate({ email: text(form, 'email'), password: text(form, 'password') })
  }

  const oidc = methods.data?.oidc
  return (
    <main className="mx-auto max-w-sm space-y-6 pt-10">
      <h1 className="text-2xl font-semibold">{t.signIn.title}</h1>
      {methods.isPending && <p className="text-muted-foreground">{t.connecting}</p>}
      {methods.isError && (
        <p role="alert" className="text-destructive">
          {methods.error instanceof TypeError ? t.unreachable : t.serverFailed}
        </p>
      )}
      {methods.data?.password && (
        <form onSubmit={submit} className="space-y-4">
          <label className="block space-y-1">
            <span className="text-sm font-medium">{t.signIn.email}</span>
            <input
              name="email"
              type="email"
              autoComplete="username"
              required
              className={inputClass}
            />
          </label>
          <label className="block space-y-1">
            <span className="text-sm font-medium">{t.signIn.password}</span>
            <input
              name="password"
              type="password"
              autoComplete="current-password"
              required
              className={inputClass}
            />
          </label>
          {login.isError && (
            <p role="alert" className="text-destructive">
              {problem(t, login.error)}
            </p>
          )}
          <Button type="submit" size="lg" className="w-full" disabled={login.isPending}>
            {login.isPending ? t.signIn.submitting : t.signIn.submit}
          </Button>
        </form>
      )}
      {methods.data?.password && oidc && (
        <p className="text-center text-sm text-muted-foreground">{t.signIn.or}</p>
      )}
      {oidc && (
        // A link, not a form: the provider's pages come after redirects,
        // which a form-action in the CSP would stop.
        <a
          href={`${oidc.login_url}?return_to=${encodeURIComponent(target)}`}
          className={buttonVariants({ variant: 'outline', size: 'lg', className: 'w-full' })}
        >
          {t.signIn.with(oidc.name)}
        </a>
      )}
      {methods.data && !methods.data.password && !oidc && (
        <p className="text-muted-foreground">{t.signIn.noMethod}</p>
      )}
    </main>
  )
}
