import { cn } from "cn";
import type { FC } from "react";

/**
 * Lightbulb glyph used for the "Other" admin empty-state badge. Rendered with
 * `currentColor`, so set the color via a text utility (e.g. `text-highlight-sky`)
 * and the size via a size utility (e.g. `size-9`).
 */
export const LightbulbIcon: FC<React.ComponentProps<"svg">> = ({
	className,
	...props
}) => (
	<svg
		viewBox="0 0 36 36"
		fill="none"
		xmlns="http://www.w3.org/2000/svg"
		aria-hidden="true"
		className={cn(className)}
		{...props}
	>
		<path d="M18 16.2L18 12.6" stroke="currentColor" strokeWidth="2.4" />
		<path
			d="M11.7002 32.625H22.9502"
			stroke="currentColor"
			strokeWidth="2.403"
			strokeLinejoin="round"
		/>
		<path
			d="M7.44881 19.5817C8.19654 21.1052 9.27978 22.4394 10.6172 23.4841L12.5998 26.1V28.8H22.4998V26.3411L24.4983 23.4701C26.357 22.0107 27.709 20.0031 28.3624 17.732C29.0158 15.4609 28.9375 13.0417 28.1384 10.8177C27.3394 8.59366 25.8603 6.67773 23.911 5.34174C21.9616 4.00574 19.6411 3.31754 17.2786 3.37476C11.1825 3.51539 6.26623 8.59054 6.29998 14.688C6.30829 16.3851 6.70107 18.0582 7.44881 19.5817Z"
			stroke="currentColor"
			strokeWidth="2.403"
			strokeLinecap="round"
		/>
	</svg>
);
