// Vendored from @shadcn/react 0.3.0 (MIT; see LICENSE in site/src/vendor).
// Upstream: https://github.com/shadcn-ui/ui/tree/b1c580c/packages/react/src/message-scroller
// Local deviations must be marked LOCAL CHANGE.

import {
  MessageScrollerButton as Button,
  MessageScrollerContent as Content,
  MessageScrollerItem as Item,
  MessageScrollerProvider as Provider,
  MessageScroller as Root,
  MessageScrollerViewport as Viewport,
} from "./components"

export const MessageScroller = {
  Provider,
  Root,
  Viewport,
  Content,
  Item,
  Button,
}

export {
  useMessageScroller,
  useMessageScrollerScrollable,
  useMessageScrollerVisibility,
} from "./components"

export type {
  MessageScrollerDefaultScrollPosition,
  MessageScrollerScrollAlign,
  MessageScrollerScrollOptions,
  MessageScrollerScrollable,
  MessageScrollerVisibilityState,
} from "./types"
