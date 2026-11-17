import { cn } from "cn";
import { OctagonAlertIcon, TriangleAlertIcon } from "lucide-react";
import type { UsageSeverity } from "#/utils/budget";

type SeverityIndicator = {
	badge: string;
	icon: React.JSX.Element;
	label: string;
};

/**
 * Badge colors, icon, and screen reader label per elevated AI spend severity.
 * `normal` has no entry, so callers render no badge for it.
 */
export const severityIndicators: Partial<
	Record<UsageSeverity, SeverityIndicator>
> = {
	warning: {
		badge: "bg-surface-orange text-highlight-orange",
		icon: <TriangleAlertIcon aria-hidden className="size-3" />,
		label: "AI spend is nearing its limit",
	},
	exceeded: {
		badge: "bg-surface-red text-highlight-red",
		icon: <OctagonAlertIcon aria-hidden className="size-3" />,
		label: "AI spend limit exceeded",
	},
};

type SeverityBadgeProps = {
	indicator: SeverityIndicator;
	className?: string;
	children?: React.ReactNode;
};

/**
 * Callers place the badge with `className`. `children` render after the icon,
 * for example screen reader text when no nearby label names the severity.
 */
export const SeverityBadge: React.FC<SeverityBadgeProps> = ({
	indicator,
	className,
	children,
}) => {
	return (
		<span
			className={cn(
				"flex size-icon-sm shrink-0 items-center justify-center rounded",
				indicator.badge,
				className,
			)}
		>
			{indicator.icon}
			{children}
		</span>
	);
};
