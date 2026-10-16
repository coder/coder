import { cva } from "class-variance-authority";
import { cn } from "cn";
import {
	ChevronRightIcon,
	CircleCheckIcon,
	InfoIcon,
	OctagonXIcon,
	TriangleAlertIcon,
	XIcon,
} from "lucide-react";
import { useState } from "react";
import { Button } from "#/components/Button/Button";
import {
	Collapsible,
	CollapsibleContent,
	CollapsibleTrigger,
} from "#/components/Collapsible/Collapsible";
import { CopyButton } from "#/components/CopyButton/CopyButton";

const alertVariants = cva(
	`relative flex w-full items-start gap-2 rounded-md border border-solid p-4
	text-left text-xs leading-4.5 font-normal text-content-primary shadow-sm`,
	{
		variants: {
			variant: {
				default: "border-border bg-surface-secondary",
				warning: "border-border-warning bg-surface-orange",
				error: "border-border-destructive bg-surface-destructive",
			},
			compact: {
				true: "min-h-13.5 items-center",
				false: "",
			},
		},
		defaultVariants: {
			variant: "default",
			compact: false,
		},
	},
);

const severityIcons = {
	info: { icon: InfoIcon, className: "text-highlight-sky" },
	success: { icon: CircleCheckIcon, className: "text-content-success" },
	warning: { icon: TriangleAlertIcon, className: "text-content-warning" },
	error: { icon: OctagonXIcon, className: "text-content-destructive" },
} as const;

export type AlertColor = "info" | "success" | "warning" | "error";

/**
 * A labeled row in an error alert's "Technical details" disclosure. Labels
 * must be unique within one alert.
 */
export type AlertDetail = Readonly<{
	label: string;
	value: string;
}>;

export type AlertProps = {
	/**
	 * Buttons or links rendered below the message. In a compact alert this
	 * renders inline after the text and should be a single `Link`.
	 */
	actions?: React.ReactNode;
	dismissible?: boolean;
	onDismiss?: () => void;
	severity?: AlertColor;
	/**
	 * Reserved for high-risk warnings and crucial errors. Ignored for `info`
	 * and `success`, which have no prominent style.
	 */
	prominent?: boolean;
	/** For one-line messages. Actions render inline after the text. */
	compact?: boolean;
	/**
	 * Rows shown in a collapsed "Technical details" disclosure. Only rendered
	 * for non-compact `error` alerts.
	 */
	details?: readonly AlertDetail[];
	children?: React.ReactNode;
	className?: string;
};

export const Alert: React.FC<AlertProps> = ({
	children,
	actions,
	dismissible,
	severity = "info",
	prominent = false,
	compact = false,
	details,
	onDismiss,
	className,
	...props
}) => {
	const [open, setOpen] = useState(true);

	if (!open) {
		return null;
	}

	const { icon: Icon, className: iconClassName } = severityIcons[severity];
	const variant =
		prominent && (severity === "warning" || severity === "error")
			? severity
			: "default";
	const showDetails =
		!compact &&
		severity === "error" &&
		details !== undefined &&
		details.length > 0;

	return (
		<div
			role="alert"
			className={cn(alertVariants({ variant, compact }), className)}
			{...props}
		>
			<span className="flex size-icon-sm shrink-0 items-center justify-center">
				<Icon className={cn("size-4", iconClassName)} />
			</span>
			{compact ? (
				<div className="flex min-w-0 flex-1 flex-wrap items-center gap-x-2">
					<div className="min-w-0">{children}</div>
					{actions}
				</div>
			) : (
				<div className="flex min-w-0 flex-1 flex-col gap-2">
					<div className="min-w-0">{children}</div>
					{showDetails && <AlertDetails details={details} />}
					{actions && (
						<div className="flex flex-wrap items-center gap-2">{actions}</div>
					)}
				</div>
			)}
			{dismissible && (
				<Button
					variant="subtle"
					size="icon"
					className="-m-1.5 size-auto p-1.5 [&>svg]:p-0"
					onClick={() => {
						setOpen(false);
						onDismiss?.();
					}}
					data-testid="dismiss-banner-btn"
					aria-label="Dismiss"
				>
					<XIcon />
				</Button>
			)}
		</div>
	);
};

const AlertDetails: React.FC<{ details: readonly AlertDetail[] }> = ({
	details,
}) => {
	const copyText = details
		.map(({ label, value }) => `${label}: ${value}`)
		.join("\n");

	return (
		<Collapsible className="flex flex-col gap-2">
			<CollapsibleTrigger
				className="group inline-flex w-fit cursor-pointer items-center gap-1 rounded-sm
				border-0 bg-transparent p-0 font-sans text-xs leading-4.5 font-normal
				text-content-primary focus-visible:outline-hidden focus-visible:ring-2
				focus-visible:ring-content-link"
			>
				<ChevronRightIcon className="size-icon-xs transition-transform group-data-[state=open]:rotate-90" />
				Technical details
			</CollapsibleTrigger>
			<CollapsibleContent className="relative rounded-sm bg-surface-primary p-2">
				<dl className="m-0 flex flex-col gap-2 pr-6 font-mono font-medium text-content-secondary">
					{details.map(({ label, value }) => (
						<div key={label} className="flex gap-2">
							<dt className="w-18 shrink-0">{label}</dt>
							<dd className="m-0 min-w-0 flex-1 whitespace-pre-wrap wrap-break-word">
								{value}
							</dd>
						</div>
					))}
				</dl>
				<CopyButton
					text={copyText}
					label="Copy technical details"
					className="absolute top-2 right-2 size-icon-sm p-0 [&>svg]:size-icon-xs [&>svg]:p-0"
				/>
			</CollapsibleContent>
		</Collapsible>
	);
};

export const AlertDescription: React.FC<
	React.PropsWithChildren<{ className?: string }>
> = ({ children, className }) => {
	return (
		<span
			className={cn("m-0 block text-xs leading-4.5 font-normal", className)}
			data-pixel="ignore"
		>
			{children}
		</span>
	);
};

export const AlertTitle: React.FC<React.ComponentProps<"h2">> = ({
	className,
	children,
	...props
}) => {
	return (
		<h2
			className={cn(
				"m-0 mb-1 text-xs leading-4.5 font-semibold last:mb-0",
				className,
			)}
			{...props}
		>
			{children}
		</h2>
	);
};
