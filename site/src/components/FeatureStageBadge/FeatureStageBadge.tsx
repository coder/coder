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
const featureStageBadgeTypes = {
	early_access: "Early Access",
	beta: "Beta",
} as const satisfies Record<string, ReactNode>;

type FeatureStageBadgeProps = Readonly<
	Omit<HTMLAttributes<HTMLSpanElement>, "children"> & {
		contentType: keyof typeof featureStageBadgeTypes;
		labelText?: string;
		size?: "xs" | "sm" | "md";
	}
>;

const badgeColorClasses = {
	early_access: "border-border-pending bg-surface-sky text-highlight-sky",
	beta: "bg-surface-sky text-highlight-sky",
} as const;

const badgeSizeClasses = {
	early_access: {
		xs: "rounded-[5px] px-1.5 py-0.5 text-2xs font-normal leading-4",
		sm: "rounded-[5px] px-2 py-0.5 text-[10px] font-normal leading-4",
		md: "rounded-[5px] px-[7px] py-[3.5px] text-xs font-normal leading-4",
	},
	beta: {
		xs: "text-2xs font-normal px-1.5 py-0.5 h-[18px] rounded border-0",
		sm: "text-xs font-medium px-2 py-1",
		md: "text-base px-2 py-1",
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
						"block max-w-fit cursor-default flex-shrink-0 leading-none whitespace-nowrap rounded-md border border-solid border-transparent transition-colors duration-200 ease-in-out",
						sizeClasses,
						colorClasses,
						className,
					)}
					{...delegatedProps}
				>
					<span className="sr-only">
						{` (This is ${contentType === "early_access" ? "an" : "a"} `}
					</span>
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
