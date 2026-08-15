import type { ElementType } from "react";

import { cn } from "#/utils/cn";

interface TextShimmerProps {
	children: string;
	as?: ElementType;
	className?: string;
	spread?: number;
}

/**
 * Text with a periodic highlight sweep, rendered entirely with a CSS
 * animation so no JavaScript runs per frame. The `shimmer` keyframes
 * sweep the highlight across the text and then hold, so the browser
 * only repaints during the sweep portion of each cycle.
 *
 * The static background position parks the highlight off the text, so
 * `motion-reduce` users see plain secondary-colored text.
 */
const ShimmerComponent = ({
	children,
	as: Component = "p",
	className,
	spread = 2,
}: TextShimmerProps) => {
	const dynamicSpread = (children?.length ?? 0) * spread;

	return (
		<Component
			data-pixel="ignore"
			className={cn(
				"relative inline-block bg-[length:250%_100%,auto] bg-clip-text text-transparent",
				"[--bg:linear-gradient(90deg,#0000_calc(50%-var(--spread)),hsl(var(--surface-primary)),#0000_calc(50%+var(--spread)))] [background-repeat:no-repeat,padding-box]",
				"[background-position:100%_center] animate-shimmer motion-reduce:animate-none",
				className,
			)}
			style={{
				"--spread": `${dynamicSpread}px`,
				backgroundImage:
					"var(--bg), linear-gradient(hsl(var(--content-secondary)), hsl(var(--content-secondary)))",
			}}
		>
			{children}
		</Component>
	);
};

export const Shimmer = ShimmerComponent;
