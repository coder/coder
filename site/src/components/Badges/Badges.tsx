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

export const EnabledBadge: FC = () => {
	return (
		<Badge className="option-enabled" variant="green" border="solid">
			Enabled
		</Badge>
	);
};

export const EntitledBadge: FC = () => {
	return (
		<Badge border="solid" variant="green">
			Entitled
		</Badge>
	);
};

interface HealthyBadgeProps {
	derpOnly?: boolean;
}

export const HealthyBadge: FC<HealthyBadgeProps> = ({ derpOnly }) => {
	return (
		<Badge variant="green" border="solid">
			{derpOnly ? "Healthy (DERP only)" : "Healthy"}
		</Badge>
	);
};

export const NotHealthyBadge: FC = () => {
	return (
		<Badge variant="destructive" border="solid">
			Unhealthy
		</Badge>
	);
};

export const NotRegisteredBadge: FC = () => {
	return (
		<Tooltip>
			<TooltipTrigger asChild>
				<Badge variant="warning" border="solid">
					Never seen
				</Badge>
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
				<Badge variant="warning" border="solid">
					Not reachable
				</Badge>
			</TooltipTrigger>
			<TooltipContent side="bottom" className="max-w-xs">
				Workspace Proxy not responding to http(s) requests.
			</TooltipContent>
		</Tooltip>
	);
};

export const DisabledBadge: FC = forwardRef<
	HTMLDivElement,
	HTMLAttributes<HTMLDivElement>
>((props, ref) => {
	return (
		<Badge ref={ref} {...props} className="option-disabled">
			Disabled
		</Badge>
	);
});

export const EnterpriseBadge: FC = () => {
	return (
		<Badge variant="info" border="solid">
			Enterprise
		</Badge>
	);
};

interface PremiumBadgeProps {
	children?: React.ReactNode;
}

export const PremiumBadge: FC<PremiumBadgeProps> = ({
	children = "Premium",
}) => {
	return (
		<Badge variant="purple" border="solid">
			{children}
		</Badge>
	);
};

export const PreviewBadge: FC = () => {
	return (
		<Badge variant="purple" border="solid">
			Preview
		</Badge>
	);
};

export const AlphaBadge: FC = () => {
	return (
		<Badge variant="purple" border="solid">
			Alpha
		</Badge>
	);
};

export const DeprecatedBadge: FC = () => {
	return (
		<Badge variant="warning" border="solid">
			Deprecated
		</Badge>
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
