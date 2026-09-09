import { cn } from "cn";
import type { CSSProperties, FC } from "react";

type FlipLoaderProps = {
	className?: string;
	label: string;
	/** Slot size in px; the card scales with it. Defaults to a 16px icon slot. */
	size?: number;
};

/** A card flipping between a primary and a tertiary face. */
export const FlipLoader: FC<FlipLoaderProps> = ({
	className,
	label,
	size = 16,
}) => (
	<span
		role="img"
		aria-label={label}
		className={cn("flex shrink-0 items-center justify-center", className)}
		style={
			{
				width: size,
				height: size,
				perspective: size * 3,
			} satisfies CSSProperties
		}
	>
		<span
			className="relative block animate-flip-card motion-reduce:animate-none [transform-style:preserve-3d]"
			style={{ width: size * 0.625, height: size * 0.75 }}
		>
			<span className="absolute inset-0 rounded-[1px] bg-content-primary [backface-visibility:hidden]" />
			<span className="absolute inset-0 rounded-[1px] bg-surface-tertiary [backface-visibility:hidden] [transform:rotateY(180deg)]" />
		</span>
	</span>
);
