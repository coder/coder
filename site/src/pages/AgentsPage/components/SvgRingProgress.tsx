import type { FC } from "react";
import { cn } from "#/utils/cn";

/**
 * Shared SVG ring (donut) progress indicator. Both UsageIndicator and
 * ContextUsageIndicator use this to avoid duplicating the SVG circle
 * pattern.
 */
export const SvgRingProgress: FC<{
	size: number;
	strokeWidth: number;
	percent: number;
	trackClassName?: string;
	progressClassName?: string;
	className?: string;
}> = ({
	size,
	strokeWidth,
	percent,
	trackClassName = "stroke-surface-tertiary",
	progressClassName = "stroke-current",
	className,
}) => {
	const radius = (size - strokeWidth) / 2;
	const circumference = 2 * Math.PI * radius;
	const clamped = Math.min(Math.max(percent, 0), 100);
	const offset = circumference * (1 - clamped / 100);

	return (
		<svg
			width={size}
			height={size}
			viewBox={`0 0 ${size} ${size}`}
			className={cn("-rotate-90", className)}
			aria-hidden="true"
		>
			<circle
				cx={size / 2}
				cy={size / 2}
				r={radius}
				fill="none"
				strokeWidth={strokeWidth}
				className={trackClassName}
			/>
			<circle
				cx={size / 2}
				cy={size / 2}
				r={radius}
				fill="none"
				strokeWidth={strokeWidth}
				strokeLinecap="round"
				className={cn(
					"transition-[stroke-dashoffset] duration-300 ease-out",
					progressClassName,
				)}
				style={{
					strokeDasharray: circumference,
					strokeDashoffset: offset,
				}}
			/>
		</svg>
	);
};
