import "@testing-library/jest-dom"
import { cleanup } from "@testing-library/react"
import crypto from "crypto"
import * as util from "util"
import { server } from "./src/testHelpers/server"

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

// Helper utility to fail jest tests if a console.error is logged
// Pulled from this blog post:
// https://www.benmvp.com/blog/catch-warnings-jest-tests/

// For now, I limited this to just 'error' - but failing on warnings
// would be a nice next step! We may need to filter out some noise
// from material-ui though.
const CONSOLE_FAIL_TYPES = ["error" /* 'warn' */] as const

// Throw errors when a `console.error` or `console.warn` happens
// by overriding the functions
CONSOLE_FAIL_TYPES.forEach((logType: typeof CONSOLE_FAIL_TYPES[number]) => {
  global.console[logType] = <Type>(format: string, ...args: Type[]): void => {
    throw new Error(
      `Failing due to console.${logType} while running test!\n\n${util.format(
        format,
        ...args,
      )}`,
    )
  }
})

// This is needed because we are compiling under `--isolatedModules`
export {}
