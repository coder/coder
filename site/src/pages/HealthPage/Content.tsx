import { css } from "@emotion/css";
import { useTheme } from "@emotion/react";
import { CheckCircleIcon } from "lucide-react";
import { XCircleIcon } from "lucide-react";
import { AlertCircleIcon } from "lucide-react";
import Link from "@mui/material/Link";
import type { HealthCode, HealthSeverity } from "api/typesGenerated";
import {
	type ComponentProps,
	type FC,
	type HTMLAttributes,
	type ReactElement,
	cloneElement,
	forwardRef,
} from "react";
import { docs } from "utils/docs";
import { healthyColor } from "./healthyColor";

const CONTENT_PADDING = 36;

export const Header: FC<HTMLAttributes<HTMLDivElement>> = (props) => {
	return (
		<header
			css={{
				display: "flex",
				alignItems: "center",
				justifyContent: "space-between",
				padding: `36px ${CONTENT_PADDING}px`,
			}}
			{...props}
		/>
	);
};

export const HeaderTitle: FC<HTMLAttributes<HTMLDivElement>> = (props) => {
	return (
		<h2
			css={{
				margin: 0,
				lineHeight: "1.2",
				fontSize: 20,
				fontWeight: 500,
				display: "flex",
				alignItems: "center",
				gap: 16,
			}}
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
	const Icon = severity === "error" ? AlertCircleIcon : CheckCircleIcon;

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
		<main
			css={{
				padding: `0 ${CONTENT_PADDING}px ${CONTENT_PADDING}px`,
				display: "flex",
				flexDirection: "column",
				gap: 36,
			}}
			{...props}
		/>
	);
};

export const GridData: FC<HTMLAttributes<HTMLDivElement>> = (props) => {
	return (
		<div
			css={{
				lineHeight: "1.4",
				display: "grid",
				gridTemplateColumns: "auto auto",
				gap: 12,
				columnGap: 48,
				width: "min-content",
				whiteSpace: "nowrap",
			}}
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
		<h4
			{...props}
			css={{
				fontSize: 14,
				fontWeight: 500,
				margin: 0,
				lineHeight: "1.2",
				marginBottom: 16,
			}}
		/>
	);
};

type PillProps = HTMLAttributes<HTMLDivElement> & {
	icon: ReactElement;
};

export const Pill = forwardRef<HTMLDivElement, PillProps>((props, ref) => {
	const theme = useTheme();
	const { icon, children, ...divProps } = props;

	return (
		<div
			ref={ref}
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
			{cloneElement(icon, { className: css({ width: 14, height: 14 }) })}
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
					<CheckCircleIcon css={{ color }} />
				) : (
					<XCircleIcon css={{ color }} />
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
				<span css={{ display: "block" }} key={index}>
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
