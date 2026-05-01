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

export const Header: FC<HTMLAttributes<HTMLDivElement>> = (props) => {
	return (
		<header
			className="flex items-center justify-between"
			style={{ padding: `36px ${CONTENT_PADDING}px` }}
			{...props}
		/>
	);
};

export const HeaderTitle: FC<HTMLAttributes<HTMLDivElement>> = (props) => {
	return (
		<h2
			className="m-0 leading-[1.2] text-xl font-medium flex items-center gap-4"
			{...props}
		/>
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

export const Main: FC<HTMLAttributes<HTMLDivElement>> = (props) => {
	return (
		<div
			className="flex flex-col gap-9"
			style={{ padding: `0 ${CONTENT_PADDING}px ${CONTENT_PADDING}px` }}
			{...props}
		/>
	);
};

export const GridData: FC<HTMLAttributes<HTMLDivElement>> = (props) => {
	return (
		<div
			className={`
				leading-[1.4] w-min whitespace-nowrap
				grid grid-cols-[auto_auto] gap-3 gap-x-12
			`}
			{...props}
		/>
	);
};

export const GridDataLabel: FC<HTMLAttributes<HTMLSpanElement>> = (props) => {
	return (
		<span className="text-sm font-medium text-content-secondary" {...props} />
	);
};

export const GridDataValue: FC<HTMLAttributes<HTMLSpanElement>> = (props) => {
	return <span className="text-sm text-content-primary" {...props} />;
};

export const SectionLabel: FC<HTMLAttributes<HTMLHeadingElement>> = (props) => {
	return (
		<h4 {...props} className="text-sm font-medium m-0 leading-[1.2] mb-4" />
	);
};

type PillProps = React.ComponentPropsWithRef<"div"> & {
	icon: ReactElement<HTMLAttributes<HTMLElement>>;
};

export const Pill: React.FC<PillProps> = ({ icon, children, ...divProps }) => {
	return (
		<div
			className="inline-flex items-center h-8 rounded-full border border-border text-xs font-medium p-2 gap-2 cursor-default"
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

export const Logs: FC<LogsProps> = ({ lines, ...divProps }) => {
	return (
		<div
			className="font-mono text-[13px] leading-[160%] p-6 bg-surface-secondary overflow-x-auto whitespace-pre-wrap break-all"
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
