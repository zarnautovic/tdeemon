// The barcodes a food's package carries: EAN and UPC, the long and the
// short forms.
const formats = ['ean_13', 'ean_8', 'upc_a', 'upc_e']

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
        return new own({ formats })
      }
    } catch {
      // A detector that cannot say what it reads: zxing-wasm's then.
    }
  }
  const [{ BarcodeDetector, prepareZXingModule }, { default: wasm }] = await Promise.all([
    import('barcode-detector/ponyfill'),
    import('zxing-wasm/reader/zxing_reader.wasm?url'),
  ])
  prepareZXingModule({
    overrides: {
      locateFile: (path: string, prefix: string) => (path.endsWith('.wasm') ? wasm : prefix + path),
    },
  })
  return new BarcodeDetector({ formats: ['ean_13', 'ean_8', 'upc_a', 'upc_e'] })
}
