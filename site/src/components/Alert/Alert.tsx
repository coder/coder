import { cva, type VariantProps } from "class-variance-authority";
import { Button } from "components/Button/Button";
import {
	CircleAlertIcon,
	CircleCheckIcon,
	InfoIcon,
	TriangleAlertIcon,
	XIcon,
} from "lucide-react";
import {
	type FC,
	forwardRef,
	type PropsWithChildren,
	type ReactNode,
	useState,
} from "react";
import { cn } from "utils/cn";

const alertVariants = cva(
	"relative w-full rounded-lg border border-solid p-4 text-left",
	{
		variants: {
			variant: {
				default: "border-border-default bg-surface-secondary",
				info: "border-border-pending bg-surface-secondary",
				success: "border-border-green bg-surface-green",
				warning: "border-border-warning bg-surface-orange",
				error: "border-border-destructive bg-surface-red",
			},
		},
		defaultVariants: {
			variant: "default",
		},
	},
);

const severityToVariant = {
	info: "info",
	success: "success",
	warning: "warning",
	error: "error",
} as const;

const variantIcons = {
	default: { icon: InfoIcon, className: "text-content-secondary" },
	info: { icon: InfoIcon, className: "text-highlight-sky" },
	success: { icon: CircleCheckIcon, className: "text-content-success" },
	warning: { icon: TriangleAlertIcon, className: "text-content-warning" },
	error: { icon: CircleAlertIcon, className: "text-content-destructive" },
} as const;

export type AlertColor = "info" | "success" | "warning" | "error";

export type AlertProps = {
	actions?: ReactNode;
	dismissible?: boolean;
	onDismiss?: () => void;
	severity?: AlertColor;
	children?: ReactNode;
	className?: string;
} & VariantProps<typeof alertVariants>;

export const Alert: FC<AlertProps> = ({
	children,
	actions,
	dismissible,
	severity = "info",
	onDismiss,
	className,
	variant,
	...props
}) => {
	const [open, setOpen] = useState(true);

	if (!open) {
		return null;
	}

	// Use severity to determine variant if variant is not explicitly provided
	const finalVariant =
		variant ||
		(severity in severityToVariant ? severityToVariant[severity] : "default");

	const { icon: Icon, className: iconClassName } = variantIcons[finalVariant];

	return (
		<div
			role="alert"
			className={cn(alertVariants({ variant: finalVariant }), className)}
			{...props}
		>
			<div className="flex items-start justify-between gap-4 text-sm">
				<div className="flex flex-row items-start gap-3">
					<Icon
						className={cn(
							"size-icon-sm mt-1",
							iconClassName,
						)}
					/>
					<div className="flex-1">{children}</div>
				</div>
				<div className="flex items-start gap-2">
					{actions}

					{dismissible && (
						<Button
							variant="subtle"
							size="icon"
							className="mt-px !size-auto !min-w-0 !p-0"
							onClick={() => {
								setOpen(false);
								onDismiss?.();
							}}
							data-testid="dismiss-banner-btn"
							aria-label="Dismiss"
						>
							<XIcon className="!size-icon-sm !p-0" />
						</Button>
					)}
				</div>
			</div>
		</div>
	);
};

export const AlertDetail: FC<PropsWithChildren> = ({ children }) => {
	return (
		<span className="m-0 text-sm" data-chromatic="ignore">
			{children}
		</span>
	);
};

export const AlertTitle = forwardRef<
	HTMLHeadingElement,
	React.HTMLAttributes<HTMLHeadingElement>
>(({ className, ...props }, ref) => (
	<h1
		ref={ref}
		className={cn(
			"m-0 mb-1 text-sm font-medium",
			className,
		)}
		{...props}
	/>
));
