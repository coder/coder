import { cva, type VariantProps } from "class-variance-authority";
import { Button } from "components/Button/Button";
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
				default: "border-border-default",
				info: "border-highlight-sky",
				success: "border-surface-green",
				warning: "border-border-warning",
				error: "border-border-destructive",
			},
		},
		defaultVariants: {
			variant: "default",
		},
	}
)

// Map MUI severity to our variant
const severityToVariant = {
	info: "info",
	success: "success",
	warning: "warning",
	error: "error",
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

	return (
		<div
			role="alert"
			className={cn(alertVariants({ variant: finalVariant }), className)}
			{...props}
		>
			<div className="flex items-start justify-between text-sm">
				<div className="flex-1">{children}</div>
				<div className="flex items-center gap-2 ml-4">
					{/* CTAs passed in by the consumer */}
					{actions}

					{dismissible && (
						<Button
							variant="subtle"
							size="sm"
							onClick={() => {
								setOpen(false);
								onDismiss?.();
							}}
							data-testid="dismiss-banner-btn"
						>
							Dismiss
						</Button>
					)}
				</div>
			</div>
		</div>
	);
};

export const AlertDetail: FC<PropsWithChildren> = ({ children }) => {
	return (
		<span className="text-sm opacity-75" data-chromatic="ignore">
			{children}
		</span>
	);
};

// Export AlertTitle and AlertDescription for compatibility
export const AlertTitle = forwardRef<
	HTMLHeadingElement,
	React.HTMLAttributes<HTMLHeadingElement>
>(({ className, ...props }, ref) => (
	<h5
		ref={ref}
		className={cn("mb-1 font-medium leading-none tracking-tight", className)}
		{...props}
	/>
));

AlertTitle.displayName = "AlertTitle";
