import { Link } from '@tanstack/react-router'

import { $api } from '@/api/client'
import { useMessages } from '@/i18n/messages'

// TakenBy says that another food of the user's has a name and brand, or
// an other name, and which, with a link to it.
export function TakenBy({ foodId, by }: { foodId: string; by: 'name' | 'alias' | 'barcode' }) {
  const t = useMessages()
  const other = $api.useQuery('get', '/api/v1/foods/{id}', { params: { path: { id: foodId } } })
  const name = other.data
    ? [other.data.name, other.data.brand].filter(Boolean).join(' · ')
    : t.foods.thatFood
  return (
    <>
      {{ name: t.foods.nameTaken, alias: t.foods.aliasTaken, barcode: t.foods.barcodeTaken }[by]}{' '}
      <Link to="/foods/$id" params={{ id: foodId }} className="underline underline-offset-2">
        {name}
      </Link>
      {other.data?.archived && ` (${t.foods.archivedMark})`}
    </>
  )
}
