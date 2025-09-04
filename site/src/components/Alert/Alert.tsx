import { cva, type VariantProps } from "class-variance-authority";
import { Button } from "components/Button/Button";
import { Alert as ShadcnAlert, AlertDescription } from "components/ui/alert";
import { AlertCircle, CheckCircle, Info, XCircle } from "lucide-react";
import {
	type FC,
	type PropsWithChildren,
	type ReactNode,
	useState,
} from "react";
import { cn } from "utils/cn";

// Map MUI severity types to our variants
export type AlertColor = "error" | "warning" | "info" | "success";

const alertVariants = cva(
	"relative w-full rounded-lg border px-4 py-3 text-sm transition-all duration-200",
	{
		variants: {
			variant: {
				info: "bg-surface-sky border-border-sky text-content-primary [&>svg]:text-highlight-sky",
				success: "bg-surface-green border-border-green text-content-primary [&>svg]:text-highlight-green",
				warning: "bg-surface-orange border-border-warning text-content-primary [&>svg]:text-highlight-orange",
				error: "bg-surface-red border-border-destructive text-content-primary [&>svg]:text-highlight-red",
			},
		},
		defaultVariants: {
			variant: "info",
		},
	},
);

const getIcon = (severity: AlertColor) => {
	switch (severity) {
		case "error":
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

export interface AlertProps {
	children: ReactNode;
	actions?: ReactNode;
	dismissible?: boolean;
	onDismiss?: () => void;
	severity?: AlertColor;
	className?: string;
	"data-testid"?: string;
}

export const Alert: FC<AlertProps> = ({
	children,
	actions,
	dismissible,
	severity = "info",
	onDismiss,
	className,
	...props
}) => {
	const [open, setOpen] = useState(true);

	// Can't only rely on hiding behavior inside flex layouts, because even
	// though the alert will have zero height when dismissed, it will
	// still behave as a flex child and introduce extra row/column gaps
	if (!open) {
		return null;
	}

	const IconComponent = getIcon(severity);

	return (
		<div
			role="alert"
			className={cn(alertVariants({ variant: severity }), className)}
			{...props}
		>
			<div className="flex items-start gap-3">
				<IconComponent className="size-4 shrink-0 mt-0.5" />
				<div className="flex-1 min-w-0">
					<AlertDescription className="text-sm leading-relaxed">
						{children}
					</AlertDescription>
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
		</div>
	);
};

export const AlertDetail: FC<PropsWithChildren> = ({ children }) => {
	return (
		<span className="text-xs text-content-secondary" data-chromatic="ignore">
			{children}
		</span>
	);
};

export const AlertTitle: FC<PropsWithChildren> = ({ children }) => {
	return (
		<h5 className="mb-1 font-medium leading-none tracking-tight text-sm">
			{children}
		</h5>
	);
};