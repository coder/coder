import { cn } from "cn";
import type { CSSProperties, FC } from "react";

type FlipLoaderProps = {
	className?: string;
	label: string;
	/** Slot size in px; the box scales with it. Defaults to a 16px icon slot. */
	size?: number;
};

const SLICES = 4;
// Phase lead per slice from the top down. Slices realign during the hold on
// each face, so the box appears to twist through the turn and snap straight.
const SLICE_LEAD_MS = 60;

/**
 * A box turning a quarter at a time, alternating a primary face and a
 * quaternary face. The faces are laid out in 3D so both are visible mid-turn.
 */
export const FlipLoader: FC<FlipLoaderProps> = ({
	className,
	label,
	size = 16,
}) => {
	const width = size * 0.625;
	const height = size * 0.75;
	const depth = width;
	const sliceHeight = height / SLICES;
	const face = "absolute inset-0 [backface-visibility:hidden]";
	return (
		<span
			role="img"
			aria-label={label}
			className={cn(
				"flex shrink-0 flex-col items-center justify-center [transform-style:preserve-3d]",
				className,
			)}
			style={
				{
					width: size,
					height: size,
					perspective: size * 2.5,
				} satisfies CSSProperties
			}
		>
			{Array.from({ length: SLICES }, (_, index) => (
				// Bands overlap by a pixel so anti-aliasing never opens a seam. The
				// pivot sits half the depth back so a resting face lands at z=0 and
				// rasterises 1:1 instead of at a perspective-scaled fraction.
				<span
					key={index}
					className="relative block [transform-style:preserve-3d]"
					style={{
						width,
						height: sliceHeight + (index > 0 ? 1 : 0),
						marginTop: index > 0 ? -1 : 0,
						transform: `translateZ(${-depth / 2}px)`,
					}}
				>
					<span
						className="absolute inset-0 animate-flip-card motion-reduce:animate-none [transform-style:preserve-3d]"
						style={{
							animationDelay: `-${(SLICES - 1 - index) * SLICE_LEAD_MS}ms`,
						}}
					>
						<span
							className={cn(face, "bg-content-primary")}
							style={{ transform: `translateZ(${depth / 2}px)` }}
						/>
						<span
							className={cn(face, "bg-surface-quaternary")}
							style={{ transform: `rotateY(90deg) translateZ(${width / 2}px)` }}
						/>
						<span
							className={cn(face, "bg-content-primary")}
							style={{
								transform: `rotateY(180deg) translateZ(${depth / 2}px)`,
							}}
						/>
						<span
							className={cn(face, "bg-surface-quaternary")}
							style={{
								transform: `rotateY(-90deg) translateZ(${width / 2}px)`,
							}}
						/>
					</span>
				</span>
			))}
		</span>
	);
};
