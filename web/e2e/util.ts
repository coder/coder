import { Page } from "@playwright/test"

/**
 * `timeout(x)` is a helper function to create a promise that resolves after `x` milliseconds.
 *
 * @param timeoutInMilliseconds Time to wait for promise to resolve
 * @returns `Promise`
 */
export const timeout = (timeoutInMilliseconds: number): Promise<void> => {
  return new Promise((resolve) => {
    setTimeout(resolve, timeoutInMilliseconds)
  })
}

/**
 * `waitFor(f, timeout?)` waits for a predicate to return `true`, running it periodically until it returns `true`.
 *
 * If `f` never returns `true`, the function will simply return. In other words, the burden is on the consumer
 * to check that the predicate is passing (`waitFor` does no validation).
 *
 * @param f A predicate that returns a `Promise<boolean>`
 * @param timeToWaitInMilliseconds  The total time to wait for the condition to be `true`.
 * @returns
 */
export const waitFor = async (
  f: () => Promise<boolean>,
  timeToWaitInMilliseconds = 30000,
): Promise<void> => {
  let elapsedTime = 0
  const timeToWaitPerIteration = 1000

  while (elapsedTime < timeToWaitInMilliseconds) {
    const condition = await f()

    if (condition) {
      return
    }

    await timeout(timeToWaitPerIteration)
    elapsedTime += timeToWaitPerIteration
  }
}

interface WaitForClientSideNavigationOpts {
  /**
   * from is the page before navigation (the 'current' page)
   */
  from?: string
  /**
   * to is the page after navigation (the 'next' page)
   */
  to?: string
}

/**
 * waitForClientSideNavigation waits for the url to change from opts.from to
 * opts.to (if specified), as well as a network idle load state. This enhances
 * a native playwright check for navigation or loadstate.
 *
 * @remark This is necessary in a client-side SPA world since playwright
 * waitForNavigation waits for load events on the DOM (ex: after a page load
 * from the server).
 */
export const waitForClientSideNavigation = async (
  page: Page,
  opts: WaitForClientSideNavigationOpts,
): Promise<void> => {
  console.info(`--- waitForClientSideNavigation: start`)

  await Promise.all([
    waitFor(() => {
      const conditions: boolean[] = []

      if (opts.from) {
        conditions.push(page.url() !== opts.from)
      }

      if (opts.to) {
        conditions.push(page.url() === opts.to)
      }

      const unmetConditions = conditions.filter((condition) => !condition)
      console.info(`--- waitForClientSideNavigation: ${unmetConditions.length} conditions not met`)

      return Promise.resolve(unmetConditions.length === 0)
    }),
    page.waitForLoadState("networkidle"),
  ])

  console.info(`--- waitForClientSideNavigation: done`)
}
