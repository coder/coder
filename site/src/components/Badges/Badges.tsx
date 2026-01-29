import { Badge } from "components/Badge/Badge";
import { Stack } from "components/Stack/Stack";
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

const classNames = {
	root: cn(
		"text-[10px] h-6 font-semibold uppercase tracking-[0.085em]",
		"px-3 flex items-center w-fit whitespace-nowrap rounded-sm",
		"rounded-full border border-solid border-zinc-700 text-white",
	),
	green: "border-green-500 bg-surface-green",
	error: "border-red-600 bg-surface-red",
	warn: "border-amber-300 bg-amber-950",
	purple: "border-violet-500 bg-surface-purple",
	info: "border-blue-400 bg-blue-950",
	orange: "border-orange-500 bg-orange-950",
};

export const EnabledBadge: FC = () => {
	return (
		<span className={cn("option-enabled", classNames.root, classNames.green)}>
			Enabled
		</span>
	);
};

export const EntitledBadge: FC = () => {
	return (
		<span className={cn(classNames.root, classNames.green)}>Entitled</span>
	);
};

interface HealthyBadgeProps {
	derpOnly?: boolean;
}

export const HealthyBadge: FC<HealthyBadgeProps> = ({ derpOnly }) => {
	return (
		<span className={cn(classNames.root, classNames.green)}>
			{derpOnly ? "Healthy (DERP only)" : "Healthy"}
		</span>
	);
};

export const NotHealthyBadge: FC = () => {
	return (
		<span className={cn(classNames.root, classNames.error)}>Unhealthy</span>
	);
};

export const NotRegisteredBadge: FC = () => {
	return (
		<Tooltip>
			<TooltipTrigger asChild>
				<span className={cn(classNames.root, classNames.warn)}>Never seen</span>
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
				<span className={cn(classNames.root, classNames.warn)}>
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
			className={cn("option-disabled", classNames.root)}
		>
			Disabled
		</span>
	);
});

export const EnterpriseBadge: FC = () => {
	return (
		<span className={cn(classNames.root, classNames.info)}>Enterprise</span>
	);
};

interface PremiumBadgeProps {
	children?: React.ReactNode;
}

export const PremiumBadge: FC<PremiumBadgeProps> = ({
	children = "Premium",
}) => {
	return (
		<Badge
			size="sm"
			className={cn(classNames.root, classNames.purple, "border-border-purple")}
		>
			{children}
		</Badge>
	);
};

export const PreviewBadge: FC = () => {
	return (
		<span className={cn(classNames.root, classNames.purple)}>Preview</span>
	);
};

export const AlphaBadge: FC = () => {
	return <span className={cn(classNames.root, classNames.purple)}>Alpha</span>;
};

export const DeprecatedBadge: FC = () => {
	return (
		<span className={cn(classNames.root, classNames.orange)}>Deprecated</span>
	);
};

export const Badges: FC<PropsWithChildren> = ({ children }) => {
	return (
		<Stack
			css={{ margin: "0 0 16px" }}
			direction="row"
			alignItems="center"
			spacing={1}
		>
			{children}
		</Stack>
	);
};
