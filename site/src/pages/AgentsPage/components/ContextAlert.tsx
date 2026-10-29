import { cn } from "cn";
import {
	CircleAlertIcon,
	InfoIcon,
	type LucideIcon,
	TriangleAlertIcon,
} from "lucide-react";
import { createContext, useContext } from "react";

type ContextAlertSeverity = "info" | "warning" | "error";

const ContextAlertSeverityContext =
	createContext<ContextAlertSeverity>("warning");
const severityIcons = {
	info: InfoIcon,
	warning: TriangleAlertIcon,
	error: CircleAlertIcon,
};

type ContextAlertProps = React.ComponentProps<"div"> & {
	severity?: ContextAlertSeverity;
};

/** A compact context notice with a severity-colored title. */
export const ContextAlert: React.FC<ContextAlertProps> = ({
	severity = "warning",
	className,
	...props
}) => (
	<ContextAlertSeverityContext.Provider value={severity}>
		<div
			role={severity === "info" ? "status" : "alert"}
			data-severity={severity}
			className={cn(
				"group/context-alert grid grid-cols-[0.75rem_minmax(0,1fr)] gap-x-1.5 gap-y-1.5",
				className,
			)}
			{...props}
		/>
	</ContextAlertSeverityContext.Provider>
);

type ContextAlertTitleProps = React.ComponentProps<"h3"> & {
	icon?: LucideIcon;
};

/** Aligns the notice heading with its severity icon. */
export const ContextAlertTitle: React.FC<ContextAlertTitleProps> = ({
	icon,
	className,
	children,
	...props
}) => {
	const severity = useContext(ContextAlertSeverityContext);
	const Icon = icon ?? severityIcons[severity];
	return (
		<h3
			className={cn(
				"col-span-2 m-0 grid grid-cols-[inherit] items-start gap-x-[inherit] text-xs font-medium text-content-warning group-data-[severity=info]/context-alert:text-content-secondary group-data-[severity=error]/context-alert:text-content-destructive",
				className,
			)}
			{...props}
		>
			<Icon aria-hidden="true" className="mt-0.5 size-3" />
			<span>{children}</span>
		</h3>
	);
};

/** Aligns notice content with the heading text rather than its icon. */
export const ContextAlertDescription: React.FC<React.ComponentProps<"div">> = ({
	className,
	...props
}) => (
	<div
		className={cn(
			"col-start-2 flex min-w-0 flex-col gap-1.5 font-normal text-content-secondary",
			className,
		)}
		{...props}
	/>
);

/** Groups actions beneath a notice's description. */
export const ContextAlertActions: React.FC<React.ComponentProps<"div">> = ({
	className,
	...props
}) => (
	<div
		className={cn("col-start-2 flex flex-wrap items-center gap-2", className)}
		{...props}
	/>
);
