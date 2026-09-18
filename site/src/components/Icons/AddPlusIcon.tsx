import { cn } from "cn";
import type { FC } from "react";

/**
 * Plus glyph used inside add/create empty-state CTAs. Rendered with
 * `currentColor`; set the color via a text utility (e.g. `text-highlight-sky`).
 * Defaults to an 18x18 size and forces it with `!` so it wins over the button's
 * default icon sizing.
 */
export const AddPlusIcon: FC<React.ComponentProps<"svg">> = ({
	className,
	...props
}) => (
	<svg
		viewBox="0 0 21 21"
		fill="none"
		xmlns="http://www.w3.org/2000/svg"
		aria-hidden="true"
		className={cn("size-icon-sm!", className)}
		{...props}
	>
		<path
			d="M10.2012 1.20154V19.2015"
			stroke="currentColor"
			strokeWidth="2.403"
			strokeLinecap="square"
			strokeLinejoin="round"
		/>
		<path
			d="M19.2012 10.2015H1.20117"
			stroke="currentColor"
			strokeWidth="2.403"
			strokeLinecap="square"
			strokeLinejoin="round"
		/>
	</svg>
);
