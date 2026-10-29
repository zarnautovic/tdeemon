import { useEffect, useEffectEvent, useId, useRef, useState } from 'react'

import { Button } from '@/components/ui/button'
import { useMessages } from '@/i18n/messages'
import { barcodeReader } from '@/lib/barcode'
import { fieldLabel, inputClass } from '@/lib/styles'
import { cn } from '@/lib/utils'

// How often a frame of the camera is read: often enough to catch a code
// held still, seldom enough for a phone's battery.
const readEvery = 300

// What the camera does: starts, reads, or is not there to read (refused,
// or a browser or a device without one).
type Camera = 'starting' | 'reading' | 'refused' | 'none'

// cameraOf is the browser's camera, when it has one to give: none on an
// origin without HTTPS, nor in a browser without the API.
function cameraOf(): MediaDevices | undefined {
  return 'mediaDevices' in navigator ? navigator.mediaDevices : undefined
}

// Scanner reads a barcode off the camera, the back one of a phone, or
// takes its digits typed: onCode gets the first read or the digits sent,
// once. The camera stops when the scanner goes.
export function Scanner({ onCode }: { onCode: (code: string) => void }) {
  const t = useMessages()
  const id = useId()
  const video = useRef<HTMLVideoElement>(null)
  const [camera, setCamera] = useState<Camera>(() => (cameraOf() ? 'starting' : 'none'))
  const [typed, setTyped] = useState('')
  const [wrong, setWrong] = useState(false)
  const done = useRef(false)

  function found(code: string) {
    if (!done.current) {
      done.current = true
      onCode(code)
    }
  }
  const read = useEffectEvent(found)

  useEffect(() => {
    let stream: MediaStream | undefined
    let timer: ReturnType<typeof setTimeout> | undefined
    // Whether the scanner is gone, which a wait of the camera's may
    // outlast: asked anew after each.
    const run = { stopped: false }
    const gone = () => run.stopped
    const devices = cameraOf()
    if (devices === undefined) {
      return undefined
    }
    async function start(media: MediaDevices) {
      try {
        stream = await media.getUserMedia({
          video: { facingMode: { ideal: 'environment' } },
          audio: false,
        })
      } catch (error) {
        if (!gone()) {
          setCamera(
            error instanceof DOMException && error.name === 'NotAllowedError' ? 'refused' : 'none',
          )
        }
        return
      }
      const v = video.current
      if (gone() || v === null) {
        return
      }
      v.srcObject = stream
      await v.play().catch(() => undefined)
      const reader = await barcodeReader()
      setCamera('reading')
      const frame = async () => {
        if (gone()) {
          return
        }
        const codes = v.readyState >= 2 ? await reader.detect(v).catch(() => []) : []
        const code = codes.map((c) => c.rawValue).find((raw) => /^\d{8,14}$/.test(raw))
        if (code === undefined) {
          timer = setTimeout(() => void frame(), readEvery)
        } else if (!gone()) {
          read(code)
        }
      }
      void frame()
    }
    void start(devices)
    return () => {
      run.stopped = true
      clearTimeout(timer)
      for (const track of stream?.getTracks() ?? []) {
        track.stop()
      }
    }
  }, [])

  return (
    <div className="space-y-4">
      {camera === 'starting' || camera === 'reading' ? (
        <div className="relative overflow-hidden rounded-lg bg-black">
          {/* Inline and muted, or iOS opens it full screen, or not at all. */}
          <video
            ref={video}
            playsInline
            muted
            autoPlay
            aria-label={t.scan.camera}
            className="aspect-[4/3] w-full object-cover"
          />
          {/* Where to hold the code. */}
          <div
            aria-hidden
            className="pointer-events-none absolute inset-x-8 top-1/2 h-24 -translate-y-1/2 rounded-md border-2 border-white/80"
          />
          <p
            aria-live="polite"
            className="absolute inset-x-0 bottom-0 bg-black/50 p-2 text-center text-small text-white"
          >
            {camera === 'reading' ? t.scan.holdStill : t.scan.starting}
          </p>
        </div>
      ) : (
        <output className="block text-sm text-muted-foreground">
          {camera === 'refused' ? t.scan.refused : t.scan.noCamera}
        </output>
      )}
      <form
        noValidate
        className="space-y-1"
        onSubmit={(e) => {
          e.preventDefault()
          const code = typed.replaceAll(/\s/g, '')
          if (!/^\d{8,14}$/.test(code)) {
            setWrong(true)
            return
          }
          found(code)
        }}
      >
        <label htmlFor={`${id}-code`} className={cn(fieldLabel, 'block')}>
          {t.scan.typeIt}
        </label>
        <div className="flex gap-2">
          <input
            id={`${id}-code`}
            value={typed}
            inputMode="numeric"
            autoComplete="off"
            aria-invalid={wrong}
            aria-describedby={wrong ? `${id}-wrong` : undefined}
            className={cn(inputClass, 'min-w-0 flex-1 tabular-nums')}
            onChange={(e) => {
              setTyped(e.currentTarget.value)
              setWrong(false)
            }}
          />
          <Button type="submit" variant="outline">
            {t.scan.lookUp}
          </Button>
        </div>
        {wrong && (
          <p id={`${id}-wrong`} className="text-xs text-destructive">
            {t.scan.digits}
          </p>
        )}
      </form>
    </div>
  )
}
