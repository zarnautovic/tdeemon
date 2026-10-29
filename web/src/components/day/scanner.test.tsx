import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import { Scanner } from '@/components/day/scanner'
import type * as Barcode from '@/lib/barcode'

// The reader the scanner gets, whose codes a test sets, and whether it
// fails to load or to read.
const reader = vi.hoisted(() => ({
  codes: [] as { rawValue: string }[],
  load: true,
  reads: true,
}))

vi.mock('@/lib/barcode', async (original) => ({
  ...(await original<typeof Barcode>()),
  barcodeReader: () =>
    reader.load
      ? Promise.resolve({
          detect: () =>
            reader.reads ? Promise.resolve(reader.codes) : Promise.reject(new Error('no WASM')),
        })
      : Promise.reject(new Error('no chunk')),
}))

// A camera's stream, whose track says when it stops.
function stream() {
  const track = { stop: vi.fn<() => void>() }
  return { track, stream: { getTracks: () => [track] } }
}

beforeEach(() => {
  // jsdom plays nothing: a video is ready at once.
  vi.spyOn(HTMLMediaElement.prototype, 'play').mockResolvedValue()
  vi.spyOn(HTMLMediaElement.prototype, 'readyState', 'get').mockReturnValue(4)
})

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
  Reflect.deleteProperty(navigator, 'mediaDevices')
  reader.codes = []
  reader.load = true
  reader.reads = true
})

function withCamera(getUserMedia: (constraints: MediaStreamConstraints) => Promise<unknown>) {
  Object.defineProperty(navigator, 'mediaDevices', { configurable: true, value: { getUserMedia } })
}

test('the back camera reads a barcode, once, and stops when the scanner goes', async () => {
  const { track, stream: s } = stream()
  const getUserMedia = vi.fn<(constraints: MediaStreamConstraints) => Promise<unknown>>(() =>
    Promise.resolve(s),
  )
  withCamera(getUserMedia)
  // A QR code first, which no food has, then the barcode.
  reader.codes = [{ rawValue: 'https://example.com' }]
  const onCode = vi.fn<(code: string) => void>()
  const { unmount } = render(<Scanner onCode={onCode} />)

  expect(await screen.findByText('Hold the barcode in the frame.')).toBeDefined()
  expect(getUserMedia).toHaveBeenCalledWith({
    video: { facingMode: { ideal: 'environment' } },
    audio: false,
  })
  act(() => {
    reader.codes = [{ rawValue: '4006381333931' }]
  })
  await waitFor(() => {
    expect(onCode).toHaveBeenCalledWith('4006381333931')
  })
  // Typed after, it is not sent again.
  fireEvent.change(screen.getByLabelText('Or type the digits under the bars'), {
    target: { value: '96385074' },
  })
  fireEvent.click(screen.getByRole('button', { name: 'Look up the code' }))
  expect(onCode).toHaveBeenCalledOnce()
  unmount()
  expect(track.stop).toHaveBeenCalled()
})

test('a camera refused says so, and the digits are typed', async () => {
  withCamera(() => Promise.reject(new DOMException('no', 'NotAllowedError')))
  const onCode = vi.fn<(code: string) => void>()
  render(<Scanner onCode={onCode} />)

  expect(
    await screen.findByText('The camera is not allowed here. Type the digits instead.'),
  ).toBeDefined()
  fireEvent.change(screen.getByLabelText('Or type the digits under the bars'), {
    target: { value: '036000 291452' },
  })
  fireEvent.click(screen.getByRole('button', { name: 'Look up the code' }))
  expect(onCode).toHaveBeenCalledWith('0036000291452')
})

test('a camera that starts after the scanner went is stopped then', async () => {
  const { track, stream: s } = stream()
  let started: ((camera: unknown) => void) | undefined
  withCamera(
    () =>
      new Promise((resolve) => {
        started = resolve
      }),
  )
  const { unmount } = render(<Scanner onCode={() => undefined} />)
  unmount()
  started?.(s)
  await waitFor(() => {
    expect(track.stop).toHaveBeenCalled()
  })
})

// brokenReader says to type the digits of a reader that fails, and
// gives what stops the camera's track.
async function brokenReader() {
  const { track, stream: s } = stream()
  withCamera(() => Promise.resolve(s))
  render(<Scanner onCode={() => undefined} />)
  expect(
    await screen.findByText('The barcode reader did not load. Type the digits instead.'),
  ).toBeDefined()
  return track.stop
}

test('a reader that does not load stops the camera and says to type the digits', async () => {
  reader.load = false
  expect(await brokenReader()).toHaveBeenCalled()
})

test('a reader that cannot read stops the camera and says to type the digits', async () => {
  reader.reads = false
  expect(await brokenReader()).toHaveBeenCalled()
})

test('digits typed are sent as the barcode they are, or said wrong', () => {
  const onCode = vi.fn<(code: string) => void>()
  render(<Scanner onCode={onCode} />)
  const field = screen.getByLabelText('Or type the digits under the bars')
  fireEvent.change(field, { target: { value: '4006381333932' } })
  fireEvent.keyDown(field, { key: 'Enter' })
  expect(screen.getByText('Those digits do not add up: check them.')).toBeDefined()
  // A UPC-A, as the EAN-13 the server keeps.
  fireEvent.change(field, { target: { value: '036000291452' } })
  fireEvent.keyDown(field, { key: 'Enter' })
  expect(onCode).toHaveBeenCalledWith('0036000291452')
})
