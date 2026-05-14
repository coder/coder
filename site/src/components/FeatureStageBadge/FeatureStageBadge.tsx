import type { FC, HTMLAttributes, ReactNode } from "react";
import { Link } from "#/components/Link/Link";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { cn } from "#/utils/cn";
import { docs } from "#/utils/docs";

/**
 * All types of feature that we are currently supporting. Defined as record to
 * ensure that we can't accidentally make typos when writing the badge text.
 */
export const featureStageBadgeTypes = {
	early_access: "Early Access",
	beta: "beta",
} as const satisfies Record<string, ReactNode>;

type FeatureStageBadgeProps = Readonly<
	Omit<HTMLAttributes<HTMLSpanElement>, "children"> & {
		contentType: keyof typeof featureStageBadgeTypes;
		labelText?: string;
		size?: "sm" | "md";
	}
>;

const badgeColorClasses = {
	early_access:
		"border-border-early-access bg-surface-early-access text-content-early-access",
	beta: "bg-surface-sky text-highlight-sky",
} as const;

const badgeSizeClasses = {
	early_access: {
		sm: "rounded-[5px] border px-2 py-0.5 text-[10px] font-normal leading-4",
		md: "rounded-[5px] border px-[7px] py-[3.5px] text-xs font-normal leading-4",
	},
	beta: {
		sm: "rounded-md border text-xs font-medium px-2 py-1",
		md: "rounded-md border text-base px-2 py-1",
	},
} as const;

export const FeatureStageBadge: FC<FeatureStageBadgeProps> = ({
	contentType,
	labelText = "",
	size = "md",
	className,
	...delegatedProps
}) => {
	const colorClasses = badgeColorClasses[contentType];
	const sizeClasses = badgeSizeClasses[contentType][size];

	return (
		<Tooltip>
			<TooltipTrigger asChild>
				<span
					className={cn(
						"block max-w-fit cursor-default flex-shrink-0 leading-none whitespace-nowrap border-solid border-transparent transition-colors duration-200 ease-in-out",
						sizeClasses,
						colorClasses,
						className,
					)}
					{...delegatedProps}
				>
					<span className="sr-only"> (This is a</span>
					<span className="first-letter:uppercase">
						{labelText && `${labelText} `}
						{featureStageBadgeTypes[contentType]}
					</span>
					<span className="sr-only"> feature)</span>
				</span>
			</TooltipTrigger>
			<TooltipContent align="start" className="max-w-xs text-sm">
				<p className="m-0">
					This feature has not yet reached general availability (GA).
				</p>

				<Link
					href={docs("/install/releases/feature-stages")}
					className="font-semibold"
				>
					Learn about feature stages
					<span className="sr-only"> (link opens in new tab)</span>
				</Link>
			</TooltipContent>
		</Tooltip>
	);
};
