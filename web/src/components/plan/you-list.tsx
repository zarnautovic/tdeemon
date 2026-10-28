import { NotebookPen, ShieldCheck, UserRound } from 'lucide-react'

import { $api } from '@/api/client'
import { ListGroup } from '@/components/list-group'
import { ListRow } from '@/components/list-row'
import { useLimitWords } from '@/components/plan/proposal-text'
import { useFormat } from '@/i18n/format'
import { useMessages } from '@/i18n/messages'

// YouList is what a plan rests on of the user's, each a row to its page
// with its value in short: the profile (or what a plan still needs of
// it), the limits (how fast a loss may go, and the least protein, as they
// hold) and the notes for a coach (their first line).
export function YouList() {
  const t = useMessages()
  const f = useFormat()
  const me = $api.useQuery('get', '/api/v1/me')
  const limits = $api.useQuery('get', '/api/v1/limits')
  const limit = useLimitWords()
  const profile = me.data?.profile
  const notes = me.data?.notes.trim().split('\n')[0]
  const holds = limits.data?.effective

  let profileSummary
  if (profile !== undefined) {
    profileSummary =
      profile.missing.length > 0
        ? t.plan.missing(profile.missing.map((m) => t.plan.missingField[m]).join(', '))
        : [
            ...(profile.sex === undefined ? [] : [t.plan.sexes[profile.sex]]),
            ...(profile.age === undefined ? [] : [t.plan.age(profile.age)]),
            ...(profile.height_cm === undefined ? [] : [`${f.number(profile.height_cm, 1)} cm`]),
          ].join(' · ')
  }

  return (
    <ListGroup title={t.plan.you}>
      <ListRow
        icon={UserRound}
        title={t.plan.profile}
        to="/plan/profile"
        summary={profileSummary}
        warn={profile !== undefined && profile.missing.length > 0}
      />
      <ListRow
        icon={ShieldCheck}
        title={t.plan.limits}
        to="/plan/limits"
        summary={
          holds?.cut_rate === undefined || holds.protein_floor === undefined
            ? undefined
            : t.plan.limitsSummary(
                limit.value('cut_rate', holds.cut_rate),
                limit.value('protein_floor', holds.protein_floor),
              )
        }
      />
      <ListRow
        icon={NotebookPen}
        title={t.settings.notes}
        to="/plan/notes"
        summary={
          me.data === undefined
            ? undefined
            : notes === undefined || notes === ''
              ? t.settings.noNotes
              : notes
        }
      />
    </ListGroup>
  )
}
