/**
 * Vendored from @shadcn/react 0.3.0 (MIT). Keep this file in sync with
 * https://github.com/shadcn-ui/ui/tree/b1c580c/packages/react/src/message-scroller
 * except for changes marked as LOCAL CHANGE.
 */
import * as React from "react";

function useLatest<T>(value: T) {
	const ref = React.useRef(value);

	ref.current = value;

	return ref;
}

export { useLatest };
