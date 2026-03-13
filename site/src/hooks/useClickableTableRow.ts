/**
 * Exposes click handlers that make table rows feel clickable without changing
 * native table semantics. Primary navigation must still be rendered as an
 * explicit in-cell link or button.
 */
import type { TableRowProps } from "@mui/material/TableRow";
import type { MouseEventHandler } from "react";
import { cn } from "utils/cn";
import { useEffectEvent } from "./hookPolyfills";
import type { UseClickableResult } from "./useClickable";

type UseClickableTableRowResult = TableRowProps &
	Omit<
		UseClickableResult<HTMLTableRowElement>,
		"role" | "tabIndex" | "onKeyDown" | "onKeyUp" | "ref"
	> & {
		className: string;
		hover: true;
		onAuxClick: MouseEventHandler<HTMLTableRowElement>;
	};

// Awkward type definition (the hover preview in VS Code isn't great, either),
// but this basically extracts all click props from TableRowProps, but makes
// onClick required, and adds additional optional props (notably onMiddleClick)
type UseClickableTableRowConfig = {
	[Key in keyof TableRowProps as Key extends `on${string}Click`
		? Key
		: never]: UseClickableTableRowResult[Key];
} & {
	onClick: MouseEventHandler<HTMLTableRowElement>;
	onMiddleClick?: MouseEventHandler<HTMLTableRowElement>;
};

export const useClickableTableRow = ({
	onClick,
	onDoubleClick,
	onMiddleClick,
	onAuxClick: externalOnAuxClick,
}: UseClickableTableRowConfig): UseClickableTableRowResult => {
	const stableOnClick = useEffectEvent(onClick);

	return {
		onClick: stableOnClick,
		className: cn([
			"cursor-pointer hover:outline focus:outline outline-1 -outline-offset-1 outline-border-hover",
			"first:rounded-t-md last:rounded-b-md",
		]),
		hover: true,
		onDoubleClick,
		onAuxClick: (event) => {
			// Regardless of which callback gets called, the hook won't stop the event
			// from bubbling further up the DOM
			const isMiddleMouseButton = event.button === 1;
			if (isMiddleMouseButton) {
				onMiddleClick?.(event);
			} else {
				externalOnAuxClick?.(event);
			}
		},
	};
};
