import { type FC, type HTMLAttributes, useRef } from "react";
import { cn } from "#/utils/cn";

interface AnimatedNumberProps extends HTMLAttributes<HTMLSpanElement> {
	/** The numeric value (or formatted string) to display. */
	value: number | string;
}

/**
 * Renders each character of a number as an individually animated span.
 * Only characters that actually changed since the last render animate;
 * stable digits stay put. Changed characters slide up and fade in with
 * a bouncy overshoot easing.
 *
 * Uses tabular-nums so digit widths are fixed and the layout does not
 * shift as values change.
 */
export const AnimatedNumber: FC<AnimatedNumberProps> = ({
	value,
	className,
	...props
}) => {
	const str = String(value);
	const chars = str.split("");

	// Track a generation counter per character position. When the
	// character at a position changes the counter bumps, giving
	// that span a new React key so it remounts and replays its
	// enter animation. Stable characters keep the same key.
	const prevStr = useRef("");
	const gens = useRef<number[]>([]);

	if (str !== prevStr.current) {
		const prev = prevStr.current.split("");
		gens.current = chars.map((char, i) => {
			const g = gens.current[i] ?? 0;
			return char !== prev[i] ? g + 1 : g;
		});
		prevStr.current = str;
	}

	return (
		<span
			className={cn("inline-flex items-baseline tabular-nums", className)}
			{...props}
		>
			{chars.map((char, i) => (
				<span
					key={`${i}-${gens.current[i]}`}
					className="inline-block animate-in fade-in-0 slide-in-from-bottom-1 fill-mode-backwards duration-500 [animation-timing-function:cubic-bezier(0.34,1.45,0.64,1)]"
				>
					{char}
				</span>
			))}
		</span>
	);
};
