import { ArrowRightIcon, CheckIcon } from "lucide-react";
import type React from "react";
import type { FC } from "react";
import { type LinkProps, Link as RouterLink } from "react-router";
import { Button } from "#/components/Button/Button";
import { Supergraphic } from "#/components/Supergraphic/Supergraphic";
import { cn } from "#/utils/cn";

export const PREMIUM_FEATURES = [
	"High availability & workspace proxies",
	"Multi-org & role-based access control",
	"24x7 global support with SLA",
	"Unlimited Git & external auth integrations",
];

export const PREMIUM_PAGE_PATH = "/deployment/premium";
export const PREMIUM_DEFAULT_DESCRIPTION =
	"You need a Premium license to use this feature.";
export const PREMIUM_DEFAULT_HERO = "Get access with a Coder trial";

export type PaywallProps = React.ComponentProps<"div"> & {
	message: string;
	description?: string;
	compact?: boolean;
	canViewPremium: boolean;
	features?: string[];
	onCTAClick?: () => void;
};

export const Paywall = ({
	className,
	children,
	...props
}: React.ComponentProps<"div">) => {
	return (
		<div
			className={cn(
				"relative isolate overflow-hidden",
				"flex flex-row items-center justify-center min-h-[280px] p-4 rounded-lg gap-8",
				"border border-solid border-border-default bg-surface-secondary",
				className,
			)}
			{...props}
		>
			{children}
		</div>
	);
};

export const PaywallContent: FC<React.ComponentProps<"div">> = ({
	children,
	className,
	...props
}) => {
	return (
		<div
			className={cn(
				"flex w-1/2 flex-col items-center justify-center px-6 text-center",
				className,
			)}
			{...props}
		>
			{children}
		</div>
	);
};

export const PaywallHeading: FC<React.ComponentProps<"div">> = ({
	children,
	className,
	...props
}) => {
	return (
		<div
			className={cn(
				"flex flex-row gap-4 items-center justify-center mb-6",
				className,
			)}
			{...props}
		>
			{children}
		</div>
	);
};

export const PaywallTitle: FC<React.ComponentProps<"h5">> = ({
	children,
	className,
	...props
}) => {
	return (
		<h5
			className={cn("font-semibold font-inherit text-xl mr-4", className)}
			{...props}
		>
			{children}
		</h5>
	);
};

export const PaywallDescription: FC<React.ComponentProps<"p">> = ({
	children,
	className,
	...props
}) => {
	return (
		<p
			className={cn("font-inherit max-w-md text-sm mb-4 mr-4", className)}
			{...props}
		>
			{children}
		</p>
	);
};

export const PaywallSupergraphic: FC<React.ComponentProps<"div">> = ({
	className,
	...props
}) => {
	return <Supergraphic className={cn("left-0 w-1/2", className)} {...props} />;
};

export const PaywallStack: FC<React.ComponentProps<"div">> = ({
	children,
	className,
	...props
}) => {
	return (
		<div
			className={cn("flex flex-1 flex-col items-start gap-6", className)}
			{...props}
		>
			{children}
		</div>
	);
};

export const PaywallFeatures: FC<React.ComponentProps<"ul">> = ({
	children,
	className,
	...props
}) => {
	return (
		<ul
			className={cn("list-none m-0 px-6 text-sm font-medium", className)}
			{...props}
		>
			{children}
		</ul>
	);
};

export const PaywallFeature: FC<React.ComponentProps<"li">> = ({
	children,
	className,
	...props
}) => {
	return (
		<li className={cn("flex items-center gap-2 p-[3px]", className)} {...props}>
			<FeatureIcon className="shrink-0" />
			<span className="flex-1">{children}</span>
		</li>
	);
};

export const PaywallCTALink: FC<LinkProps> = ({
	children,
	className,
	...props
}) => {
	return (
		<Button asChild>
			<RouterLink className={cn("mx-7", className)} {...props}>
				<ArrowRightIcon aria-hidden="true" />
				{children}
			</RouterLink>
		</Button>
	);
};

export const PaywallGuidance: FC<React.ComponentProps<"p">> = ({
	children,
	className,
	...props
}) => {
	return (
		<p
			className={cn(
				"font-inherit text-sm max-w-[280px] mx-7 my-0 text-content-secondary",
				className,
			)}
			{...props}
		>
			{children}
		</p>
	);
};

const FeatureIcon: FC<React.ComponentProps<"svg">> = ({
	className,
	...props
}) => {
	return (
		<CheckIcon
			aria-hidden="true"
			className={cn("size-icon-sm", className)}
			{...props}
		/>
	);
};
