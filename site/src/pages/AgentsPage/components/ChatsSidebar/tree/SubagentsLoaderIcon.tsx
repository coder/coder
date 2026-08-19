import type { ComponentProps, JSX } from "react";

/**
 * Spinner variant used when a chat has subagents: lucide's LoaderCircle
 * arc plus a second, inner arc so the icon reads as multiple agents
 * working at once. The arcs rotate independently (the inner one
 * counter-rotates at a slower speed) so the motion conveys concurrent
 * work rather than a single rigid spinner. Drawn on the same 24x24 grid
 * and stroke settings as lucide icons so it can substitute for one.
 */
export const SubagentsLoaderIcon = (
	props: ComponentProps<"svg">,
): JSX.Element => (
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
		<path
			className="origin-center [transform-box:view-box] animate-spin"
			d="M21 12a9 9 0 1 1-6.219-8.56"
		/>
		<path
			className="origin-center [transform-box:view-box] animate-[spin_1.5s_linear_infinite_reverse]"
			d="M16.5 12a4.5 4.5 0 1 1-3.11-4.28"
		/>
	</svg>
);
