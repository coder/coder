import type { FC } from "react";
import {
	clampPercentage,
	severityProgressClassName,
	type UsageSeverity,
} from "#/utils/budget";
import { cn } from "#/utils/cn";

interface UsageBarProps {
	/** Fraction used, 0-100. Clamped for safety. */
	percent: number;
	severity?: UsageSeverity;
	ariaLabel: string;
	/** Track overrides, e.g. height. */
	className?: string;
}

export const UsageBar: FC<UsageBarProps> = ({
	percent,
	severity = "normal",
	ariaLabel,
	className,
}) => {
	const clampedPercent = clampPercentage(percent);

	return (
		<div
			role="progressbar"
			aria-label={ariaLabel}
			aria-valuemin={0}
			aria-valuemax={100}
			aria-valuenow={Math.round(clampedPercent)}
			className={cn(
				"h-1.5 overflow-hidden rounded-full bg-surface-tertiary",
				className,
			)}
		>
			<div
				className={cn(
					"h-full rounded-full transition-all duration-300 ease-out",
					severityProgressClassName(severity),
				)}
				style={{ width: `${clampedPercent}%` }}
			/>
		</div>
	);
};
