import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import { Scanner } from '@/components/day/scanner'

// The reader the scanner gets, whose codes a test sets.
const reader = vi.hoisted(() => ({
  codes: [] as { rawValue: string }[],
}))

vi.mock('@/lib/barcode', () => ({
  barcodeReader: () => Promise.resolve({ detect: () => Promise.resolve(reader.codes) }),
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
  expect(onCode).toHaveBeenCalledWith('036000291452')
})
