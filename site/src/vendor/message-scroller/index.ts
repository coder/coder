/**
 * Vendored from @shadcn/react 0.3.0 (MIT). Keep this file in sync with
 * https://github.com/shadcn-ui/ui/tree/b1c580c/packages/react/src/message-scroller
 * except for changes marked as LOCAL CHANGE.
 */
import {
	MessageScrollerButton as Button,
	MessageScrollerContent as Content,
	MessageScrollerItem as Item,
	MessageScrollerProvider as Provider,
	MessageScroller as Root,
	MessageScrollerViewport as Viewport,
} from "./components";

export const MessageScroller = {
	Provider,
	Root,
	Viewport,
	Content,
	Item,
	Button,
};

export {
	useMessageScroller,
	useMessageScrollerScrollable,
	useMessageScrollerVisibility,
} from "./components";

export type {
	MessageScrollerDefaultScrollPosition,
	MessageScrollerScrollAlign,
	MessageScrollerScrollable,
	MessageScrollerScrollOptions,
	MessageScrollerVisibilityState,
} from "./types";
