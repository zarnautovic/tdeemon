import { Copy } from 'lucide-react'
import { useRef, useState, type Ref } from 'react'

import { Button } from '@/components/ui/button'
import { useMessages } from '@/i18n/messages'
import { fieldLabel } from '@/lib/styles'
import { cn } from '@/lib/utils'

// CopyField is a text to copy, whole, in a monospace block under its
// label, which names the two together (a code block takes no name), with
// a button that copies it in a tap and is named by the label. A browser
// that cannot copy it (no clipboard over plain http, or one refused)
// selects it, for the user to copy by hand. lines keeps its lines, as a
// file's. buttonRef is the button's, for what gives it the focus.
export function CopyField({
  label,
  value,
  lines = false,
  buttonRef,
}: {
  label: string
  value: string
  lines?: boolean
  buttonRef?: Ref<HTMLButtonElement> | undefined
}) {
  const t = useMessages()
  const [copied, setCopied] = useState<'yes' | 'failed' | undefined>(undefined)
  const code = useRef<HTMLElement>(null)

  async function copy() {
    try {
      await navigator.clipboard.writeText(value)
      setCopied('yes')
    } catch {
      if (code.current) {
        window.getSelection()?.selectAllChildren(code.current)
      }
      setCopied('failed')
    }
  }

  return (
    <fieldset className="min-w-0 space-y-1.5">
      <legend className={fieldLabel}>{label}</legend>
      <div className="flex items-start gap-2">
        <code
          ref={code}
          className={cn(
            'block min-w-0 flex-1 rounded-md bg-surface-2 p-3 font-mono text-small break-all select-all',
            lines && 'whitespace-pre-wrap',
          )}
        >
          {value}
        </code>
        <Button
          ref={buttonRef}
          variant="outline"
          size="sm"
          aria-label={copied === 'yes' ? t.settings.copiedOf(label) : t.settings.copyOf(label)}
          onClick={() => void copy()}
        >
          <Copy aria-hidden />
          {copied === 'yes' ? t.settings.copied : t.settings.copy}
        </Button>
      </div>
      {copied === 'failed' && (
        <p role="alert" className="text-small text-destructive">
          {t.settings.copyFailed}
        </p>
      )}
    </fieldset>
  )
}
