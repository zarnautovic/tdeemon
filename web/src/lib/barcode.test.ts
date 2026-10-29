import { afterEach, expect, test, vi } from 'vitest'

import { barcodeOf, barcodeReader } from '@/lib/barcode'

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

test("zxing-wasm's reader is made once, its WASM compiled once", async () => {
  vi.stubGlobal('BarcodeDetector', undefined)
  const [one, two] = await Promise.all([barcodeReader(), barcodeReader()])
  expect(one).toBe(two)
  expect(await barcodeReader()).toBe(one)
})

test('a barcode is kept as the server keeps it, or said to be none', () => {
  for (const [typed, code] of [
    ['4 006381 333931', '4006381333931'],
    ['96385074', '96385074'],
    ['036000291452', '0036000291452'],
    ['00036000291452', '0036000291452'],
    ['10036000291459', '10036000291459'],
    ['04252614', '0042100005264'],
    ['0000096385074', '96385074'],
  ] as const) {
    expect(barcodeOf(typed)).toEqual({ code })
  }
  for (const [typed, no] of [
    ['1234567', 'length'],
    ['1234567890', 'length'],
    ['4006381333932', 'check'],
    ['24252615', 'check'],
    ['400638133393x', 'length'],
  ] as const) {
    expect(barcodeOf(typed)).toEqual({ no })
  }
})
