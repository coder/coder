// Vendored from @shadcn/react 0.3.0 (MIT; see LICENSE in site/src/vendor).
// Upstream: https://github.com/shadcn-ui/ui/tree/b1c580c/packages/react/src/message-scroller
// Local deviations must be marked LOCAL CHANGE.

import * as React from "react"

function useLatest<T>(value: T) {
  const ref = React.useRef(value)

  ref.current = value

  return ref
}

export { useLatest }
