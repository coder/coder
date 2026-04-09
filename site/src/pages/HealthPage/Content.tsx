import { useTheme } from "@emotion/react";
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
import { docs } from "#/utils/docs";
import { healthyColor } from "./healthyColor";

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
				width: 8,
				height: 8,
				borderRadius: 9999,
				backgroundColor: healthyColor(theme, severity),
			}}
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
	const theme = useTheme();
	return (
		<span
			css={{
				fontSize: 14,
				fontWeight: 500,
				color: theme.palette.text.secondary,
			}}
			{...props}
		/>
	);
};

export const GridDataValue: FC<HTMLAttributes<HTMLSpanElement>> = (props) => {
	const theme = useTheme();
	return (
		<span
			css={{
				fontSize: 14,
				color: theme.palette.text.primary,
			}}
			{...props}
		/>
	);
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
	const theme = useTheme();

	return (
		<div
			css={{
				display: "inline-flex",
				alignItems: "center",
				height: 32,
				borderRadius: 9999,
				border: `1px solid ${theme.palette.divider}`,
				fontSize: 12,
				fontWeight: 500,
				padding: 8,
				gap: 8,
				cursor: "default",
			}}
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
				fontFamily: "monospace",
				fontSize: 13,
				lineHeight: "160%",
				padding: 24,
				backgroundColor: theme.palette.background.paper,
				overflowX: "auto",
				whiteSpace: "pre-wrap",
				wordBreak: "break-all",
			}}
			{...divProps}
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
			className="mx-0"
		>
			Docs for {code}
		</Link>
	);
};
