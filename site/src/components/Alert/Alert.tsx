import Collapse from "@mui/material/Collapse";
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

const alertVariants = cva("relative w-full rounded-lg border p-4 text-left", {
	variants: {
		variant: {
			default: "bg-surface-primary text-content-primary border-border-default",
			info: "bg-blue-50 text-blue-900 border-blue-200 dark:bg-blue-950 dark:text-blue-100 dark:border-blue-800",
			success:
				"bg-green-50 text-green-900 border-green-200 dark:bg-green-950 dark:text-green-100 dark:border-green-800",
			warning:
				"bg-yellow-50 text-yellow-900 border-yellow-200 dark:bg-yellow-950 dark:text-yellow-100 dark:border-yellow-800",
			error:
				"bg-red-50 text-red-900 border-red-200 dark:bg-red-950 dark:text-red-100 dark:border-red-800",
		},
	},
	defaultVariants: {
		variant: "default",
	},
});

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

	// Can't only rely on MUI's hiding behavior inside flex layouts, because even
	// though MUI will make a dismissed alert have zero height, the alert will
	// still behave as a flex child and introduce extra row/column gaps
	if (!open) {
		return null;
	}

	// Use severity to determine variant if variant is not explicitly provided
	const finalVariant =
		variant ||
		(severity in severityToVariant ? severityToVariant[severity] : "default");

	return (
		<Collapse in>
			<div
				role="alert"
				className={cn(alertVariants({ variant: finalVariant }), className)}
				{...props}
			>
				<div className="flex items-start justify-between">
					<div className="flex-1">{children}</div>
					<div className="flex items-center gap-2 ml-4">
						{/* CTAs passed in by the consumer */}
						{actions}

						{/* close CTA */}
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
		</Collapse>
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

export const AlertDescription = forwardRef<
	HTMLDivElement,
	React.HTMLAttributes<HTMLDivElement>
>(({ className, ...props }, ref) => (
	<div
		ref={ref}
		className={cn("text-sm [&_p]:leading-relaxed", className)}
		{...props}
	/>
));

AlertDescription.displayName = "AlertDescription";
