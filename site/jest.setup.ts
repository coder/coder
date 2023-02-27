import "@testing-library/jest-dom"
import { cleanup } from "@testing-library/react"
import crypto from "crypto"
import { server } from "./src/testHelpers/server"
import "jest-location-mock"
import { TextEncoder, TextDecoder } from "util"
import { Blob } from "buffer"

global.TextEncoder = TextEncoder
// eslint-disable-next-line @typescript-eslint/no-explicit-any -- Polyfill for jsdom
global.TextDecoder = TextDecoder as any
// eslint-disable-next-line @typescript-eslint/no-explicit-any -- Polyfill for jsdom
global.Blob = Blob as any

// Polyfill the getRandomValues that is used on utils/random.ts
Object.defineProperty(global.self, "crypto", {
  value: {
    getRandomValues: function (buffer: Buffer) {
      return crypto.randomFillSync(buffer)
    },
  },
})

// Establish API mocking before all tests through MSW.
beforeAll(() =>
  server.listen({
    onUnhandledRequest: "warn",
  }),
)

// Reset any request handlers that we may add during the tests,
// so they don't affect other tests.
afterEach(() => {
  cleanup()
  server.resetHandlers()
  jest.clearAllMocks()
})

// Clean up after the tests are finished.
afterAll(() => server.close())

// This is needed because we are compiling under `--isolatedModules`
export {}
