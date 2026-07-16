import type { FC } from "react";
import { cn } from "#/utils/cn";

/**
 * SVG ring (donut) progress indicator.
 *
 * The rendered SVG is aria-hidden; callers must provide an accessible
 * wrapper (e.g. a progressbar role or labeled button).
 *
 * @param percent - Fill percentage, clamped to [0, 100].
 */
export const SvgRingProgress: FC<{
	size: number;
	strokeWidth: number;
	percent: number;
	markerPercent?: number;
	trackClassName?: string;
	progressClassName?: string;
	markerClassName?: string;
	className?: string;
}> = ({
	size,
	strokeWidth,
	percent,
	markerPercent,
	trackClassName = "stroke-surface-tertiary",
	progressClassName = "stroke-current",
	markerClassName = "stroke-current",
	className,
}) => {
	const radius = (size - strokeWidth) / 2;
	const circumference = 2 * Math.PI * radius;
	const clamped = Math.min(Math.max(percent, 0), 100);
	const clampedMarker =
		typeof markerPercent === "number" &&
		Number.isFinite(markerPercent) &&
		markerPercent > 0 &&
		markerPercent < 100
			? markerPercent
			: undefined;
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
			{clampedMarker !== undefined && (
				<line
					x1={size - strokeWidth}
					y1={size / 2}
					x2={size}
					y2={size / 2}
					strokeWidth={1.25}
					strokeLinecap="butt"
					className={markerClassName}
					transform={`rotate(${clampedMarker * 3.6} ${size / 2} ${size / 2})`}
				/>
			)}
		</svg>
	);
};
