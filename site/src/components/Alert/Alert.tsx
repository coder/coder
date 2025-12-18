import { cva } from "class-variance-authority";
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
			severity: {
				info: "",
				success: "",
				warning: "",
				error: "",
			},
			prominent: {
				true: "",
				false: "",
			},
		},
		compoundVariants: [
			{
				prominent: false,
				className: "border-border-default bg-surface-secondary",
			},
			{
				severity: "success",
				prominent: true,
				className: "border-border-success bg-surface-green",
			},
			{
				severity: "warning",
				prominent: true,
				className: "border-border-warning bg-surface-orange",
			},
			{
				severity: "error",
				prominent: true,
				className: "border-border-destructive bg-surface-red",
			},
		],
		defaultVariants: {
			severity: "info",
			prominent: false,
		},
	},
);

const severityIcons = {
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
	prominent?: boolean;
	children?: ReactNode;
	className?: string;
};

export const Alert: FC<AlertProps> = ({
	children,
	actions,
	dismissible,
	severity = "info",
	prominent = false,
	onDismiss,
	className,
	...props
}) => {
	const [open, setOpen] = useState(true);

	if (!open) {
		return null;
	}

	const { icon: Icon, className: iconClassName } = severityIcons[severity];

	return (
		<div
			role="alert"
			className={cn(alertVariants({ severity, prominent }), className)}
			{...props}
		>
			<div className="flex items-start justify-between gap-4 text-sm">
				<div className="flex flex-row items-start gap-3">
					<Icon className={cn("size-icon-sm mt-1", iconClassName)} />
					<div className="flex-1">{children}</div>
				</div>
				<div className="flex items-center gap-2">
					{actions}

					{dismissible && (
						<Button
							variant="subtle"
							size="icon"
							className="!size-auto !min-w-0 !p-0"
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
		className={cn("m-0 mb-1 text-sm font-medium", className)}
		{...props}
	/>
));
