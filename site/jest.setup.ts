import "@testing-library/jest-dom"
import { cleanup } from "@testing-library/react"
import crypto from "crypto"
import { server } from "./src/testHelpers/server"
import "jest-location-mock"
import { TextEncoder, TextDecoder } from "util"
import { Blob } from "buffer"
import { fetch, Request, Response, Headers } from "@remix-run/web-fetch"

global.TextEncoder = TextEncoder
// eslint-disable-next-line @typescript-eslint/no-explicit-any -- Polyfill for jsdom
global.TextDecoder = TextDecoder as any
// eslint-disable-next-line @typescript-eslint/no-explicit-any -- Polyfill for jsdom
global.Blob = Blob as any

// From REMIX https://github.com/remix-run/react-router/blob/main/packages/react-router-dom/__tests__/setup.ts
if (!global.fetch) {
  // Built-in lib.dom.d.ts expects `fetch(Request | string, ...)` but the web
  // fetch API allows a URL so @remix-run/web-fetch defines
  // `fetch(string | URL | Request, ...)`
  // @ts-expect-error -- Polyfill for jsdom
  global.fetch = fetch
  // Same as above, lib.dom.d.ts doesn't allow a URL to the Request constructor
  // @ts-expect-error -- Polyfill for jsdom
  global.Request = Request
  // web-std/fetch Response does not currently implement Response.error()
  // @ts-expect-error -- Polyfill for jsdom
  global.Response = Response
  global.Headers = Headers
}

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
