import { cn } from "cn";
import {
	BellIcon,
	CherryIcon,
	CitrusIcon,
	GemIcon,
	StarIcon,
} from "lucide-react";
import type { FC } from "react";
import { useMediaQuery } from "#/hooks/useMediaQuery";

const reducedMotionQuery = "(prefers-reduced-motion: reduce)";

// Row 0 is the landed row and is repeated at the end so the looping reel
// animation restarts on an identical frame. The reels share one animation
// with staggered delays so they land one after another.
const reelSymbols = [CherryIcon, CitrusIcon, GemIcon, StarIcon, BellIcon];
const reelRows = [...reelSymbols, reelSymbols[0]].map((Icon, index) => ({
	id: `${index}`,
	Icon,
}));
const reelDelays = [
	"[animation-delay:0ms]",
	"[animation-delay:120ms]",
	"[animation-delay:240ms]",
];
const rowHeightRem = 1; // Matches the size-4 icon rows below.
const reelTravel = `-${reelSymbols.length * rowHeightRem}rem`;

/**
 * Easter egg. Three-reel slot machine shown in place of the Thinking
 * indicator's icon and shimmer. Renders no accessible content; a text label
 * must accompany it. With reduced motion the reels do not animate and show
 * the landed row.
 */
export const SlotMachineIndicator: FC<{ className?: string }> = ({
	className,
}) => {
	const reducedMotion = useMediaQuery(reducedMotionQuery);

	return (
		<div
			aria-hidden
			data-testid="slot-machine-indicator"
			className={cn(
				"flex h-5 items-center gap-px rounded-sm border border-border-default bg-surface-secondary px-0.5 text-content-secondary",
				className,
			)}
		>
			{reelDelays.map((delay) => (
				<div key={delay} className="size-4 overflow-hidden">
					<div
						style={{ "--slot-reel-travel": reelTravel }}
						className={cn(
							"flex flex-col",
							reducedMotion ? "animate-none" : "animate-slot-reel",
							delay,
						)}
					>
						{reelRows.map(({ id, Icon }) => (
							<Icon key={id} className="size-4 shrink-0 p-0.5 stroke-[1.75]" />
						))}
					</div>
				</div>
			))}
		</div>
	);
};
