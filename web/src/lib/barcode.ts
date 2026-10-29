// The barcodes a food's package carries: EAN and UPC, the long and the
// short forms.
const formats = ['ean_13', 'ean_8', 'upc_a', 'upc_e'] as const

// A BarcodeReader finds the barcodes in a frame of the camera.
export interface BarcodeReader {
  detect: (frame: HTMLVideoElement) => Promise<{ rawValue: string }[]>
}

interface ReaderClass {
  new (options: { formats: string[] }): BarcodeReader
  getSupportedFormats: () => Promise<string[]>
}

function isReaderClass(v: unknown): v is ReaderClass {
  return typeof v === 'function' && 'getSupportedFormats' in v
}

// zxing-wasm's reader, made once: each made anew compiles its WASM anew.
let zxing: Promise<BarcodeReader> | undefined

// barcodeReader is the browser's own BarcodeDetector where it reads the
// barcodes of food (Chrome on Android), else zxing-wasm's (Safari on an
// iPhone has none): loaded with the scanner alone, and its WASM from the
// app, not the CDN the library would ask.
export async function barcodeReader(): Promise<BarcodeReader> {
  const own: unknown = Reflect.get(globalThis, 'BarcodeDetector')
  if (isReaderClass(own)) {
    try {
      const supported = await own.getSupportedFormats()
      if (formats.every((f) => supported.includes(f))) {
        return new own({ formats: [...formats] })
      }
    } catch {
      // A detector that cannot say what it reads: zxing-wasm's then.
    }
  }
  zxing ??= (async () => {
    const [{ BarcodeDetector, prepareZXingModule }, { default: wasm }] = await Promise.all([
      import('barcode-detector/ponyfill'),
      import('zxing-wasm/reader/zxing_reader.wasm?url'),
    ])
    prepareZXingModule({
      overrides: {
        locateFile: (path: string, prefix: string) =>
          path.endsWith('.wasm') ? wasm : prefix + path,
      },
    })
    return new BarcodeDetector({ formats: [...formats] })
  })()
  // One that failed to load is tried anew next time.
  return zxing.catch((error: unknown) => {
    zxing = undefined
    throw error
  })
}

// Why digits are no barcode: not 8, 12, 13 or 14 of them, or a check
// digit that does not hold (a digit misread or mistyped).
export type NoBarcode = 'length' | 'check'

// barcodeOf is the barcode s is, in the form the server keeps it
// (food.Barcode): its spaces left out; a UPC-A, a GTIN-14 of a 0 first
// and a UPC-E as the EAN-13 they are; an EAN-8 padded with zeros as the
// EAN-8. Or why it is none.
export function barcodeOf(s: string): { code: string } | { no: NoBarcode } {
  let code = s.replaceAll(/\s/g, '')
  if (!/^(\d{8}|\d{12,14})$/.test(code)) {
    return { no: 'length' }
  }
  if (code.length === 8 && !checkDigitOK(code) && (code.startsWith('0') || code.startsWith('1'))) {
    code = upcA(code)
  }
  if (!checkDigitOK(code)) {
    return { no: 'check' }
  }
  if (code.length === 12) {
    code = `0${code}`
  } else if (code.length === 14 && code.startsWith('0')) {
    code = code.slice(1)
  }
  if (code.length === 13 && code.startsWith('00000')) {
    code = code.slice(5)
  }
  return { code }
}

// checkDigitOK reports whether the last digit of code is the check digit
// of the others: their sum, weighted 3 and 1 in turn from the right, and
// the check digit make a multiple of 10.
function checkDigitOK(code: string): boolean {
  let sum = Number(code.at(-1))
  for (let i = code.length - 2, w = 3; i >= 0; i -= 1, w = 4 - w) {
    sum += Number(code[i]) * w
  }
  return sum % 10 === 0
}

// upcA is the UPC-A a UPC-E stands for: its number system, its six digits
// with the zeros it leaves out put back as its sixth digit says, and its
// check digit.
function upcA(upce: string): string {
  const d = upce.slice(1, 7)
  const last = d.charAt(5)
  let body: string
  if (last === '0' || last === '1' || last === '2') {
    body = `${d.slice(0, 2)}${last}0000${d.slice(2, 5)}`
  } else if (last === '3') {
    body = `${d.slice(0, 3)}00000${d.slice(3, 5)}`
  } else if (last === '4') {
    body = `${d.slice(0, 4)}00000${d.charAt(4)}`
  } else {
    body = `${d.slice(0, 5)}0000${last}`
  }
  return `${upce.charAt(0)}${body}${upce.charAt(7)}`
}
