import { useEffect, useEffectEvent, useId, useRef, useState } from 'react'

import { Button } from '@/components/ui/button'
import { useMessages } from '@/i18n/messages'
import { barcodeOf, barcodeReader, type BarcodeReader, type NoBarcode } from '@/lib/barcode'
import { fieldLabel, inputClass } from '@/lib/styles'
import { cn } from '@/lib/utils'

// How often a frame of the camera is read: often enough to catch a code
// held still, seldom enough for a phone's battery.
const readEvery = 300

// What the camera does: starts, reads, or is not there to read (refused,
// a browser or a device without one, or a reader that did not load).
type Camera = 'starting' | 'reading' | 'refused' | 'none' | 'broken'

// cameraOf is the browser's camera, when it has one to give: none on an
// origin without HTTPS, nor in a browser without the API.
function cameraOf(): MediaDevices | undefined {
  return 'mediaDevices' in navigator ? navigator.mediaDevices : undefined
}

function stop(stream: MediaStream | undefined) {
  for (const track of stream?.getTracks() ?? []) {
    track.stop()
  }
}

// Scanner reads a barcode off the camera, the back one of a phone, or
// takes its digits typed: onCode gets the first read or typed, once, as
// the server keeps it (barcodeOf); sendWith is what its button says, Look
// up the code unless given. The camera stops when the scanner goes, or
// when its reader fails, which says to type the digits.
export function Scanner({
  onCode,
  sendWith,
}: {
  onCode: (code: string) => void
  sendWith?: string
}) {
  const t = useMessages()
  const id = useId()
  const video = useRef<HTMLVideoElement>(null)
  const [camera, setCamera] = useState<Camera>(() => (cameraOf() ? 'starting' : 'none'))
  const [typed, setTyped] = useState('')
  const [wrong, setWrong] = useState<NoBarcode | undefined>(undefined)
  const done = useRef(false)

  function found(code: string) {
    if (!done.current) {
      done.current = true
      onCode(code)
    }
  }
  const read = useEffectEvent(found)

  // send sends the digits typed, when they are a barcode.
  function send() {
    const b = barcodeOf(typed)
    if ('no' in b) {
      setWrong(b.no)
      return
    }
    found(b.code)
  }

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
    // The reader loads while the camera starts.
    const reading = barcodeReader().then(
      (reader) => ({ reader }),
      () => ({ reader: undefined }),
    )
    const broken = () => {
      stop(stream)
      if (!gone()) {
        setCamera('broken')
      }
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
        // Gone while the camera started: it stops now.
        stop(stream)
        return
      }
      v.srcObject = stream
      await v.play().catch(() => undefined)
      const { reader } = await reading
      if (reader === undefined) {
        broken()
        return
      }
      if (!gone()) {
        setCamera('reading')
      }
      void frames(v, reader)
    }
    async function frames(v: HTMLVideoElement, reader: BarcodeReader) {
      if (gone()) {
        return
      }
      let codes: { rawValue: string }[] = []
      if (v.readyState >= 2) {
        try {
          codes = await reader.detect(v)
        } catch {
          // A reader that cannot read reads nothing again.
          broken()
          return
        }
      }
      const code = codes
        .map((c) => barcodeOf(c.rawValue))
        .flatMap((b) => ('code' in b ? [b.code] : []))[0]
      if (code === undefined) {
        timer = setTimeout(() => void frames(v, reader), readEvery)
      } else if (!gone()) {
        stop(stream)
        read(code)
      }
    }
    void start(devices)
    return () => {
      run.stopped = true
      clearTimeout(timer)
      stop(stream)
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
          {camera === 'refused'
            ? t.scan.refused
            : camera === 'broken'
              ? t.scan.broken
              : t.scan.noCamera}
        </output>
      )}
      {/* No form of its own: it may be in a food's form. */}
      <div className="space-y-1">
        <label htmlFor={`${id}-code`} className={cn(fieldLabel, 'block')}>
          {t.scan.typeIt}
        </label>
        <div className="flex gap-2">
          <input
            id={`${id}-code`}
            value={typed}
            inputMode="numeric"
            autoComplete="off"
            aria-invalid={wrong !== undefined}
            aria-describedby={wrong ? `${id}-wrong` : undefined}
            className={cn(inputClass, 'min-w-0 flex-1 tabular-nums')}
            onChange={(e) => {
              setTyped(e.currentTarget.value)
              setWrong(undefined)
            }}
            onKeyDown={(e) => {
              if (e.key === 'Enter') {
                e.preventDefault()
                send()
              }
            }}
          />
          <Button type="button" variant="outline" onClick={send}>
            {sendWith ?? t.scan.lookUp}
          </Button>
        </div>
        {wrong && (
          <p id={`${id}-wrong`} className="text-xs text-destructive">
            {wrong === 'length' ? t.scan.digits : t.scan.check}
          </p>
        )}
      </div>
    </div>
  )
}
