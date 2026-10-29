import { afterEach, expect, test, vi } from 'vitest'

import { barcodeReader } from '@/lib/barcode'

type Prepare = (options: {
  overrides: { locateFile: (path: string, prefix: string) => string }
}) => void

// zxing-wasm's detector, and how it is told where its WASM is.
const ponyfill = vi.hoisted(() => ({
  BarcodeDetector: class {
    options: { formats: string[] }
    constructor(options: { formats: string[] }) {
      this.options = options
    }
    detect = () => Promise.resolve([])
  },
  prepareZXingModule: vi.fn<Prepare>(),
}))

vi.mock('barcode-detector/ponyfill', () => ponyfill)
vi.mock('zxing-wasm/reader/zxing_reader.wasm?url', () => ({
  default: '/assets/zxing_reader-x.wasm',
}))

afterEach(() => {
  vi.unstubAllGlobals()
  ponyfill.prepareZXingModule.mockClear()
})

// The browser's own detector, where it reads the formats given.
function own(formats: string[]) {
  return class {
    static getSupportedFormats = () => Promise.resolve(formats)
    options: { formats: string[] }
    constructor(options: { formats: string[] }) {
      this.options = options
    }
    detect = () => Promise.resolve([])
  }
}

const food = { formats: ['ean_13', 'ean_8', 'upc_a', 'upc_e'] }

test("the browser's own detector reads the barcodes of food where it can", async () => {
  const Own = own(['qr_code', 'ean_13', 'ean_8', 'upc_a', 'upc_e'])
  vi.stubGlobal('BarcodeDetector', Own)
  const reader = await barcodeReader()
  expect(reader).toBeInstanceOf(Own)
  expect(reader).toHaveProperty('options', food)
  expect(ponyfill.prepareZXingModule).not.toHaveBeenCalled()
})

test("without one, or one that reads no EAN, zxing-wasm's, its WASM the app's own", async () => {
  vi.stubGlobal('BarcodeDetector', undefined)
  const none = await barcodeReader()
  vi.stubGlobal('BarcodeDetector', own(['qr_code']))
  const noEAN = await barcodeReader()
  for (const reader of [none, noEAN]) {
    expect(reader).toBeInstanceOf(ponyfill.BarcodeDetector)
    expect(reader).toHaveProperty('options', food)
  }
  const locate = ponyfill.prepareZXingModule.mock.lastCall?.[0].overrides.locateFile
  expect(locate?.('zxing_reader.wasm', 'https://cdn.example/')).toBe('/assets/zxing_reader-x.wasm')
  expect(locate?.('other.js', '/x/')).toBe('/x/other.js')
})
