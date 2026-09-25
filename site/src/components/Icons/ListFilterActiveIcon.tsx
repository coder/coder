import type { FC, SVGProps } from "react";

/**
 * Variant of lucide's `ListFilterIcon` with a status dot on the top line,
 * used to signal that at least one filter is applied. Drawn on the same 24px
 * grid and stroke as lucide so it can swap in without layout shift.
 */
export const ListFilterActiveIcon: FC<SVGProps<SVGSVGElement>> = (props) => {
	return (
		<svg
			xmlns="http://www.w3.org/2000/svg"
			width="24"
			height="24"
			viewBox="0 0 24 24"
			fill="none"
			stroke="currentColor"
			strokeWidth="2"
			strokeLinecap="round"
			strokeLinejoin="round"
			{...props}
		>
			<path d="M3 6h9" />
			<path d="M7 12h11" />
			<path d="M10 18h5" />
			<circle cx="18" cy="6" r="3" />
		</svg>
	);
};
