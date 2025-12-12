import { useTheme } from "@emotion/react";
import Link from "@mui/material/Link";
import type { HealthCode, HealthSeverity } from "api/typesGenerated";
import {
	CircleAlertIcon,
	CircleCheckIcon,
	CircleMinusIcon,
} from "lucide-react";
import {
	type ComponentProps,
	cloneElement,
	type FC,
	forwardRef,
	type HTMLAttributes,
	type ReactElement,
} from "react";
import { cn } from "utils/cn";
import { docs } from "utils/docs";
import { healthyColor } from "./healthyColor";

const CONTENT_PADDING = 36;

export const Header: FC<HTMLAttributes<HTMLDivElement>> = (props) => {
	return (
		<header
			css={{
				padding: `36px ${CONTENT_PADDING}px`,
			}}
			className="flex items-center justify-between"
			{...props}
		/>
	);
};

export const HeaderTitle: FC<HTMLAttributes<HTMLDivElement>> = (props) => {
	return (
		<h2
			{...props}
			className={cn(
				"m-0 leading-tight text-xl font-medium flex items-center gap-4",
				props.className,
			)}
		/>
	);
};

interface HealthIconProps {
	size: number;
	severity: HealthSeverity;
}

export const HealthIcon: FC<HealthIconProps> = ({ size, severity }) => {
	const theme = useTheme();
	const color = healthyColor(theme, severity);
	const Icon = severity === "error" ? CircleAlertIcon : CircleCheckIcon;

	return <Icon css={{ width: size, height: size, color }} />;
};

interface HealthyDotProps {
	severity: HealthSeverity;
}

export const HealthyDot: FC<HealthyDotProps> = ({ severity }) => {
	const theme = useTheme();

	return (
		<div
			css={{
				backgroundColor: healthyColor(theme, severity),
			}}
			className="size-2 rounded-full"
		/>
	);
};

export const Main: FC<HTMLAttributes<HTMLDivElement>> = (props) => {
	return (
		<main
			css={{
				padding: `0 ${CONTENT_PADDING}px ${CONTENT_PADDING}px`,
			}}
			className="flex flex-col gap-9"
			{...props}
		/>
	);
};

export const GridData: FC<HTMLAttributes<HTMLDivElement>> = (props) => {
	return (
		<div
			className="leading-snug grid grid-cols-[auto_auto] gap-x-12 gap-y-3 w-min whitespace-nowrap"
			{...props}
		/>
	);
};

export const GridDataLabel: FC<HTMLAttributes<HTMLSpanElement>> = (props) => {
	const theme = useTheme();
	return (
		<span
			css={{
				color: theme.palette.text.secondary,
			}}
			className="text-sm font-medium leading-snug"
			{...props}
		/>
	);
};

export const GridDataValue: FC<HTMLAttributes<HTMLSpanElement>> = (props) => {
	const theme = useTheme();
	return (
		<span
			css={{
				color: theme.palette.text.primary,
			}}
			className="text-sm leading-none leading-snug"
			{...props}
		/>
	);
};

export const SectionLabel: FC<HTMLAttributes<HTMLHeadingElement>> = (props) => {
	return (
		<h4 {...props} className="text-sm font-medium m-0 leading-tight mb-4" />
	);
};

type PillProps = HTMLAttributes<HTMLDivElement> & {
	icon: ReactElement<HTMLAttributes<HTMLElement>>;
};

export const Pill = forwardRef<HTMLDivElement, PillProps>((props, ref) => {
	const theme = useTheme();
	const { icon, children, ...divProps } = props;

	return (
		<div
			ref={ref}
			css={{
				border: `1px solid ${theme.palette.divider}`,
			}}
			{...divProps}
			className={cn(
				"inline-flex items-center h-8 rounded-full text-xs font-medium gap-2 p-2 cursor-default",
				divProps.className,
			)}
		>
			{cloneElement(icon, { className: "size-[14px]" })}
			{children}
		</div>
	);
});

type BooleanPillProps = Omit<ComponentProps<typeof Pill>, "icon" | "value"> & {
	value: boolean | null;
};

export const BooleanPill: FC<BooleanPillProps> = ({
	value,
	children,
	...divProps
}) => {
	const theme = useTheme();
	const color = value ? theme.roles.success.outline : theme.roles.error.outline;

	return (
		<Pill
			icon={
				value ? (
					<CircleCheckIcon css={{ color }} className="size-icon-sm" />
				) : (
					<CircleMinusIcon css={{ color }} className="size-icon-sm" />
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
	const theme = useTheme();

	return (
		<div
			css={{
				backgroundColor: theme.palette.background.paper,
			}}
			{...divProps}
			className={cn(
				"font-mono text-[13px] leading-[160%] p-6 overflow-x-auto whitespace-pre-wrap break-all",
				divProps.className,
			)}
		>
			{lines.map((line, index) => (
				<span className="block" key={index}>
					{line}
				</span>
			))}
			{lines.length === 0 && (
				<span css={{ color: theme.palette.text.secondary }}>
					No logs available
				</span>
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
		>
			Docs for {code}
		</Link>
	);
};
