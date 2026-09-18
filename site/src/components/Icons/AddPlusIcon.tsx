import { cn } from "cn";
import type { FC } from "react";

/**
 * Plus glyph used as the empty-state badge icon (the visual above the headline)
 * for empty states whose CTA is an add/create action. Rendered with
 * `currentColor`; set the color and size via utilities (e.g.
 * `size-9 text-highlight-sky`).
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
		className={cn(className)}
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
