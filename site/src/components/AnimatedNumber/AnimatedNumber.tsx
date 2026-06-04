import type { FC, HTMLAttributes } from "react";
import { cn } from "#/utils/cn";

interface AnimatedNumberProps extends HTMLAttributes<HTMLSpanElement> {
	/** The numeric value (or formatted string) to display. */
	value: number | string;
}

/**
 * Renders each character of a number as an individually animated span.
 * Characters slide up and fade in with a bouncy overshoot easing; the
 * last two characters stagger behind the leading ones so trailing
 * digits feel alive without looking chaotic.
 *
 * The animation replays whenever `value` changes because each character
 * span is keyed on the stringified value, forcing React to remount them.
 */
export const AnimatedNumber: FC<AnimatedNumberProps> = ({
	value,
	className,
	...props
}) => {
	const str = String(value);
	const chars = str.split("");
	const len = chars.length;

	return (
		<span className={cn("inline-flex items-baseline", className)} {...props}>
			{chars.map((char, i) => {
				// Leading characters enter together; the last two trail
				// behind by 1x and 2x the stagger interval (70ms).
				const fromEnd = len - 1 - i;
				const delay = fromEnd < 2 ? (2 - fromEnd) * 70 : 0;

				return (
					<span
						key={`${str}-${i}`}
						className={cn(
							"inline-block",
							"animate-in fade-in-0 slide-in-from-bottom-1 fill-mode-backwards duration-500",
							"[animation-timing-function:cubic-bezier(0.34,1.45,0.64,1)]",
						)}
						style={delay > 0 ? { animationDelay: `${delay}ms` } : undefined}
					>
						{char}
					</span>
				);
			})}
		</span>
	);
};
