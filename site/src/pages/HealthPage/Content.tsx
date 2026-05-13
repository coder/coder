import {
	CircleAlertIcon,
	CircleCheckIcon,
	CircleHelpIcon,
	CircleMinusIcon,
} from "lucide-react";
import {
	type ComponentProps,
	cloneElement,
	type FC,
	type HTMLAttributes,
	type ReactElement,
} from "react";
import type { HealthCode, HealthSeverity } from "#/api/typesGenerated";
import { Link } from "#/components/Link/Link";
import { cn } from "#/utils/cn";
import { docs } from "#/utils/docs";

const CONTENT_PADDING = 36;

export const Header: FC<HTMLAttributes<HTMLDivElement>> = ({
	className,
	style,
	children,
	...props
}) => {
	return (
		<header
			className={cn("flex items-center justify-between", className)}
			style={{ padding: `36px ${CONTENT_PADDING}px`, ...style }}
			{...props}
		>
			{children}
		</header>
	);
};

export const HeaderTitle: FC<HTMLAttributes<HTMLDivElement>> = ({
	className,
	children,
	...props
}) => {
	return (
		<h2
			className={cn(
				"m-0 leading-[1.2] text-xl font-medium flex items-center gap-4",
				className,
			)}
			{...props}
		>
			{children}
		</h2>
	);
};

interface HealthIconProps {
	size: number;
	severity: HealthSeverity;
}

export const HealthIcon: FC<HealthIconProps> = ({ size, severity }) => {
	const Icon = severity === "error" ? CircleAlertIcon : CircleCheckIcon;

	return (
		<Icon
			className={cn(
				severity === "ok" && "text-content-success",
				severity === "warning" && "text-content-warning",
				severity === "error" && "text-content-destructive",
			)}
			style={{ width: size, height: size }}
		/>
	);
};

interface HealthyDotProps {
	severity: HealthSeverity;
}

export const HealthyDot: FC<HealthyDotProps> = ({ severity }) => {
	return (
		<div
			className={cn(
				"size-2 rounded-full",
				severity === "ok" && "bg-content-success",
				severity === "warning" && "bg-content-warning",
				severity === "error" && "bg-content-destructive",
			)}
		/>
	);
};

export const Main: FC<HTMLAttributes<HTMLDivElement>> = ({
	className,
	style,
	children,
	...props
}) => {
	return (
		<div
			className={cn("flex flex-col gap-9", className)}
			style={{
				padding: `0 ${CONTENT_PADDING}px ${CONTENT_PADDING}px`,
				...style,
			}}
			{...props}
		>
			{children}
		</div>
	);
};

export const GridData: FC<HTMLAttributes<HTMLDivElement>> = ({
	className,
	children,
	...props
}) => {
	return (
		<div
			className={cn(
				"leading-[1.4] w-min whitespace-nowrap",
				"grid grid-cols-[auto_auto] gap-3 gap-x-12",
				className,
			)}
			{...props}
		>
			{children}
		</div>
	);
};

export const GridDataLabel: FC<HTMLAttributes<HTMLSpanElement>> = ({
	className,
	children,
	...props
}) => {
	return (
		<span
			className={cn("text-sm font-medium text-content-secondary", className)}
			{...props}
		>
			{children}
		</span>
	);
};

export const GridDataValue: FC<HTMLAttributes<HTMLSpanElement>> = ({
	className,
	children,
	...props
}) => {
	return (
		<span className={cn("text-sm text-content-primary", className)} {...props}>
			{children}
		</span>
	);
};

export const SectionLabel: FC<HTMLAttributes<HTMLHeadingElement>> = ({
	className,
	children,
	...props
}) => {
	return (
		<h4
			className={cn("text-sm font-medium m-0 leading-[1.2] mb-4", className)}
			{...props}
		>
			{children}
		</h4>
	);
};

type PillProps = React.ComponentPropsWithRef<"div"> & {
	icon: ReactElement<HTMLAttributes<HTMLElement>>;
};

export const Pill: React.FC<PillProps> = ({
	className,
	icon,
	children,
	...divProps
}) => {
	return (
		<div
			className={cn(
				"inline-flex items-center h-8 rounded-full border border-solid border-border text-xs font-medium p-2 gap-2 cursor-default",
				className,
			)}
			{...divProps}
		>
			{cloneElement(icon, { className: "size-[14px]" })}
			{children}
		</div>
	);
};

interface StatusIconProps {
	value: boolean | null;
}

export const StatusIcon: FC<StatusIconProps> = ({ value }) => {
	if (value === null) {
		return <CircleHelpIcon className="size-icon-sm text-content-disabled" />;
	}
	return value ? (
		<CircleCheckIcon className="size-icon-sm text-content-success" />
	) : (
		<CircleMinusIcon className="size-icon-sm text-content-destructive" />
	);
};

type BooleanPillProps = Omit<ComponentProps<typeof Pill>, "icon" | "value"> & {
	value: boolean | null;
};

export const BooleanPill: FC<BooleanPillProps> = ({
	value,
	children,
	...divProps
}) => {
	return (
		<Pill
			icon={
				value ? (
					<CircleCheckIcon className="size-icon-sm text-content-success" />
				) : (
					<CircleMinusIcon className="size-icon-sm text-content-destructive" />
				)
			}
			{...divProps}
		>
			{children}
		</Pill>
	);
};

type LogsProps = HTMLAttributes<HTMLDivElement> & { lines: readonly string[] };

export const Logs: FC<LogsProps> = ({ className, lines, ...divProps }) => {
	return (
		<div
			className={cn(
				"font-mono text-[13px] leading-relaxed p-6 bg-surface-secondary overflow-x-auto whitespace-pre-wrap break-all",
				className,
			)}
			{...divProps}
		>
			{lines.map((line, index) => (
				<span className="block" key={index}>
					{line}
				</span>
			))}
			{lines.length === 0 && (
				<span className="text-content-secondary">No logs available</span>
			)}
		</div>
	);
};

interface HealthMessageDocsLinkProps {
	code: HealthCode;
}

export const HealthMessageDocsLink: FC<HealthMessageDocsLinkProps> = ({
	code,
}) => {
	return (
		<Link
			href={docs(`/admin/monitoring/health-check#${code.toLocaleLowerCase()}`)}
			target="_blank"
			rel="noreferrer"
			className="mx-0"
		>
			Docs for {code}
		</Link>
	);
};
