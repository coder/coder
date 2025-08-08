import Tooltip from "@mui/material/Tooltip";
import { type VariantProps, cva } from "class-variance-authority";
import { Stack } from "components/Stack/Stack";
import {
	type FC,
	type HTMLAttributes,
	type PropsWithChildren,
	forwardRef,
} from "react";
import { cn } from "utils/cn";

const badgeVariants = cva(
	"text-[10px] h-6 font-semibold uppercase tracking-[0.085em] px-3 rounded-full flex items-center w-fit whitespace-nowrap",
	{
		variants: {
			variant: {
				enabled:
					"border border-success-outline bg-success-background text-success-text",
				error:
					"border border-error-outline bg-error-background text-error-text",
				warn: "border border-warning-outline bg-warning-background text-warning-text",
				neutral: "border border-l1-outline bg-l1-background text-l1-text",
				enterprise:
					"border border-enterprise-border bg-enterprise-background text-enterprise-text",
				premium:
					"border border-premium-border bg-premium-background text-premium-text",
				preview:
					"border border-preview-outline bg-preview-background text-preview-text",
				danger:
					"border border-danger-outline bg-danger-background text-danger-text",
			},
		},
		defaultVariants: {
			variant: "neutral",
		},
	},
);

interface BadgeProps
	extends HTMLAttributes<HTMLSpanElement>,
		VariantProps<typeof badgeVariants> {}

const Badge = forwardRef<HTMLSpanElement, BadgeProps>(
	({ className, variant, ...props }, ref) => {
		return (
			<span
				{...props}
				ref={ref}
				className={cn(badgeVariants({ variant }), className)}
			/>
		);
	},
);

export const EnabledBadge: FC = () => {
	return (
		<Badge variant="enabled" className="option-enabled">
			Enabled
		</Badge>
	);
};

export const EntitledBadge: FC = () => {
	return <Badge variant="enabled">Entitled</Badge>;
};

interface HealthyBadge {
	derpOnly?: boolean;
}
export const HealthyBadge: FC<HealthyBadge> = ({ derpOnly }) => {
	return (
		<Badge variant="enabled">
			{derpOnly ? "Healthy (DERP only)" : "Healthy"}
		</Badge>
	);
};

export const NotHealthyBadge: FC = () => {
	return <Badge variant="error">Unhealthy</Badge>;
};

export const NotRegisteredBadge: FC = () => {
	return (
		<Tooltip title="Workspace Proxy has never come online and needs to be started.">
			<Badge variant="warn">Never seen</Badge>
		</Tooltip>
	);
};

export const NotReachableBadge: FC = () => {
	return (
		<Tooltip title="Workspace Proxy not responding to http(s) requests.">
			<Badge variant="warn">Not reachable</Badge>
		</Tooltip>
	);
};

export const DisabledBadge: FC = forwardRef<
	HTMLSpanElement,
	HTMLAttributes<HTMLSpanElement>
>((props, ref) => {
	return (
		<Badge
			{...props}
			ref={ref}
			variant="neutral"
			className={cn("option-disabled", props.className)}
		>
			Disabled
		</Badge>
	);
});

export const EnterpriseBadge: FC = () => {
	return <Badge variant="enterprise">Enterprise</Badge>;
};

export const PremiumBadge: FC = () => {
	return <Badge variant="premium">Premium</Badge>;
};

export const PreviewBadge: FC = () => {
	return <Badge variant="preview">Preview</Badge>;
};

export const AlphaBadge: FC = () => {
	return <Badge variant="preview">Alpha</Badge>;
};

export const DeprecatedBadge: FC = () => {
	return <Badge variant="danger">Deprecated</Badge>;
};

export const Badges: FC<PropsWithChildren> = ({ children }) => {
	return (
		<Stack className="mb-4" direction="row" alignItems="center" spacing={1}>
			{children}
		</Stack>
	);
};
