/**
 * Alert component following Coder's established patterns
 * Based on shadcn/ui Alert with custom variants for Coder's design system
 */
import { Slot } from "@radix-ui/react-slot";
import { cva, type VariantProps } from "class-variance-authority";
import { Button } from "components/Button/Button";
import { AlertCircle, CheckCircle, Info, XCircle } from "lucide-react";
import {
	type ReactNode,
	forwardRef,
	useState,
} from "react";
import { cn } from "utils/cn";

const alertVariants = cva(
	"relative w-full rounded-lg border px-4 py-3 text-sm transition-all duration-200",
	{
		variants: {
			variant: {
				default: "bg-surface-secondary border-border-default text-content-primary [&>svg]:text-content-secondary",
				info: "bg-surface-sky border-border-sky text-content-primary [&>svg]:text-highlight-sky",
				success: "bg-surface-green border-border-green text-content-primary [&>svg]:text-highlight-green",
				warning: "bg-surface-orange border-border-warning text-content-primary [&>svg]:text-highlight-orange",
				destructive: "bg-surface-red border-border-destructive text-content-primary [&>svg]:text-highlight-red",
			},
			size: {
				sm: "px-3 py-2 text-xs",
				md: "px-4 py-3 text-sm",
				lg: "px-6 py-4 text-base",
			},
		},
		defaultVariants: {
			variant: "default",
			size: "md",
		},
	},
);

const getIcon = (variant: string) => {
	switch (variant) {
		case "destructive":
			return XCircle;
		case "warning":
			return AlertCircle;
		case "success":
			return CheckCircle;
		case "info":
		default:
			return Info;
	}
};

export interface AlertProps
	extends React.HTMLAttributes<HTMLDivElement>,
		VariantProps<typeof alertVariants> {
	children: ReactNode;
	actions?: ReactNode;
	dismissible?: boolean;
	onDismiss?: () => void;
	asChild?: boolean;
	showIcon?: boolean;
}

export const Alert = forwardRef<HTMLDivElement, AlertProps>(
	({
		children,
		actions,
		dismissible,
		variant = "default",
		size = "md",
		onDismiss,
		className,
		asChild = false,
		showIcon = true,
		...props
	}, ref) => {
		const [open, setOpen] = useState(true);
		const Comp = asChild ? Slot : "div";

		// Can't only rely on hiding behavior inside flex layouts, because even
		// though the alert will have zero height when dismissed, it will
		// still behave as a flex child and introduce extra row/column gaps
		if (!open) {
			return null;
		}

		const IconComponent = getIcon(variant);

		return (
			<Comp
				ref={ref}
				role="alert"
				className={cn(alertVariants({ variant, size }), className)}
				{...props}
			>
				<div className="flex items-start gap-3">
					{showIcon && <IconComponent className="size-4 shrink-0 mt-0.5" />}
					<div className="flex-1 min-w-0">
						{children}
					</div>
					{(actions || dismissible) && (
						<div className="flex items-center gap-2 ml-auto">
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
					)}
				</div>
			</Comp>
		);
	},
);

Alert.displayName = "Alert";

export const AlertTitle = forwardRef<
	HTMLHeadingElement,
	React.HTMLAttributes<HTMLHeadingElement>
>(({ className, ...props }, ref) => (
	<h5
		ref={ref}
		className={cn("mb-1 font-medium leading-none tracking-tight text-sm", className)}
		{...props}
	/>
));

AlertTitle.displayName = "AlertTitle";

export const AlertDetail = forwardRef<
	HTMLDivElement,
	React.HTMLAttributes<HTMLDivElement>
>(({ className, ...props }, ref) => (
	<div
		ref={ref}
		className={cn("text-xs text-content-secondary [&_p]:leading-relaxed", className)}
		data-chromatic="ignore"
		{...props}
	/>
));

AlertDetail.displayName = "AlertDetail";
