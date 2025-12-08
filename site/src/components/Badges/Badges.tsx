import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "components/Tooltip/Tooltip";
import {
	type FC,
	forwardRef,
	type HTMLAttributes,
	type PropsWithChildren,
} from "react";
import { cn } from "utils/cn";

const badgeClasses = {
	root: [
		"text-[10px] h-6 font-semibold uppercase tracking-[0.085em]",
		"px-3 rounded-full flex items-center w-fit whitespace-nowrap",
		"border border-solid",
	],
	enabled: ["border-green-500 bg-green-950 text-green-50"],
	error: ["border-red-600 bg-red-950 text-red-50"],
	warn: ["border-amber-300 bg-amber-950 text-amber-50"],
	enterprise: ["border-blue-400 bg-blue-950 text-blue-50"],
	disabled: ["border-zinc-700 bg-zinc-900 text-white"],
	premium: ["border-violet-400 bg-violet-950 text-violet-50"],
	preview: ["border-violet-500 bg-violet-950 text-violet-50"],
	deprecated: ["border-orange-500 bg-orange-950 text-orange-50"],
} as const;

export const EnabledBadge: FC = () => {
	return (
		<span
			className={cn([
				"option-enabled",
				badgeClasses.root,
				badgeClasses.enabled,
			])}
		>
			Enabled
		</span>
	);
};

export const EntitledBadge: FC = () => {
	return (
		<span className={cn(badgeClasses.root, badgeClasses.enabled)}>
			Entitled
		</span>
	);
};

interface HealthyBadge {
	derpOnly?: boolean;
}
export const HealthyBadge: FC<HealthyBadge> = ({ derpOnly }) => {
	return (
		<span className={cn(badgeClasses.root, badgeClasses.enabled)}>
			{derpOnly ? "Healthy (DERP only)" : "Healthy"}
		</span>
	);
};

export const NotHealthyBadge: FC = () => {
	return (
		<span className={cn(badgeClasses.root, badgeClasses.error)}>Unhealthy</span>
	);
};

export const NotRegisteredBadge: FC = () => {
	return (
		<Tooltip>
			<TooltipTrigger asChild>
				<span className={cn(badgeClasses.root, badgeClasses.warn)}>
					Never seen
				</span>
			</TooltipTrigger>
			<TooltipContent side="bottom" className="max-w-xs">
				Workspace Proxy has never come online and needs to be started.
			</TooltipContent>
		</Tooltip>
	);
};

export const NotReachableBadge: FC = () => {
	return (
		<Tooltip>
			<TooltipTrigger asChild>
				<span className={cn(badgeClasses.root, badgeClasses.warn)}>
					Not reachable
				</span>
			</TooltipTrigger>
			<TooltipContent side="bottom" className="max-w-xs">
				Workspace Proxy not responding to http(s) requests.
			</TooltipContent>
		</Tooltip>
	);
};

export const DisabledBadge: FC = forwardRef<
	HTMLSpanElement,
	HTMLAttributes<HTMLSpanElement>
>((props, ref) => {
	return (
		<span
			{...props}
			ref={ref}
			className={cn([
				"option-disabled",
				badgeClasses.root,
				badgeClasses.disabled,
			])}
		>
			Disabled
		</span>
	);
});

export const EnterpriseBadge: FC = () => {
	return (
		<span className={cn(badgeClasses.root, badgeClasses.enterprise)}>
			Enterprise
		</span>
	);
};

export const PremiumBadge: FC = () => {
	return (
		<span className={cn(badgeClasses.root, badgeClasses.premium)}>Premium</span>
	);
};

export const PreviewBadge: FC = () => {
	return (
		<span className={cn(badgeClasses.root, badgeClasses.preview)}>Preview</span>
	);
};

export const AlphaBadge: FC = () => {
	return (
		<span className={cn(badgeClasses.root, badgeClasses.preview)}>Alpha</span>
	);
};

export const DeprecatedBadge: FC = () => {
	return (
		<span className={cn(badgeClasses.root, badgeClasses.deprecated)}>
			Deprecated
		</span>
	);
};

export const Badges: FC<PropsWithChildren> = ({ children }) => {
	return (
		<div className="flex flex-row items-center gap-2 mb-4">{children}</div>
	);
};
