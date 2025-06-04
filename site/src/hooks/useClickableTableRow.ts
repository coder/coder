/**
 * @file 2024-02-19 - MES - Sadly, even though this hook aims to make elements
 * more accessible, it's doing the opposite right now. Per axe audits, the
 * current implementation will create a bunch of critical-level accessibility
 * violations:
 *
 * 1. Nesting interactive elements (e.g., workspace table rows having checkboxes
 *    inside them)
 * 2. Overriding the native element's role (in this case, turning a native table
 *    row into a button, which means that screen readers lose the ability to
 *    announce the row's data as part of a larger table)
 *
 * It might not make sense to test this hook until the underlying design
 * problems are fixed.
 */
import type { TableRowProps } from "@mui/material/TableRow";
import type { MouseEventHandler } from "react";
import { cn } from "utils/cn";
import {
	type ClickableAriaRole,
	type UseClickableResult,
	useClickable,
} from "./useClickable";

type UseClickableTableRowResult<
	TRole extends ClickableAriaRole = ClickableAriaRole,
> = UseClickableResult<HTMLTableRowElement, TRole> &
	TableRowProps & {
		className: string;
		hover: true;
		onAuxClick: MouseEventHandler<HTMLTableRowElement>;
	};

// Awkward type definition (the hover preview in VS Code isn't great, either),
// but this basically extracts all click props from TableRowProps, but makes
// onClick required, and adds additional optional props (notably onMiddleClick)
type UseClickableTableRowConfig<TRole extends ClickableAriaRole> = {
	[Key in keyof TableRowProps as Key extends `on${string}Click`
		? Key
		: never]: UseClickableTableRowResult<TRole>[Key];
} & {
	role?: TRole;
	onClick: MouseEventHandler<HTMLTableRowElement>;
	onMiddleClick?: MouseEventHandler<HTMLTableRowElement>;
};

export const useClickableTableRow = <
	TRole extends ClickableAriaRole = ClickableAriaRole,
>({
	role,
	onClick,
	onDoubleClick,
	onMiddleClick,
	onAuxClick: externalOnAuxClick,
}: UseClickableTableRowConfig<TRole>): UseClickableTableRowResult<TRole> => {
	const clickableProps = useClickable(onClick, (role ?? "button") as TRole);

	return {
		...clickableProps,
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
