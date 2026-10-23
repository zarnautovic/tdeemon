import { useMessages } from '@/i18n/messages'

export function ChartsPage() {
  const t = useMessages()

  return (
    <section className="space-y-2">
      <h1 className="text-2xl font-semibold">{t.charts.title}</h1>
      <p className="text-muted-foreground">{t.soon}</p>
    </section>
  )
}
