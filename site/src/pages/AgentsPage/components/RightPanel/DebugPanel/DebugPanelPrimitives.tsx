import type { FC, ReactNode } from "react";
import { Badge } from "#/components/Badge/Badge";
import { CopyButton } from "#/components/CopyButton/CopyButton";
import { cn } from "#/utils/cn";
import { getRoleBadgeVariant, safeJsonStringify } from "./debugPanelUtils";

interface DebugDataSectionProps {
	title: string;
	description?: ReactNode;
	children: ReactNode;
	className?: string;
}

export const DebugDataSection: FC<DebugDataSectionProps> = ({
	title,
	description,
	children,
	className,
}) => {
	return (
		<section className={cn("space-y-1.5", className)}>
			<h4 className="text-xs font-medium text-content-secondary">{title}</h4>
			{description ? (
				<p className="text-xs leading-5 text-content-tertiary">{description}</p>
			) : null}
			<div>{children}</div>
		</section>
	);
};

interface DebugCodeBlockProps {
	code: string;
	className?: string;
}

const DebugCodeBlock: FC<DebugCodeBlockProps> = ({ code, className }) => {
	return (
		<pre
			className={cn(
				"w-full max-w-full max-h-[28rem] overflow-auto rounded-lg bg-surface-tertiary/60 px-3 py-2.5 font-mono text-[12px] leading-5 text-content-primary shadow-inner",
				className,
			)}
		>
			<code>{code}</code>
		</pre>
	);
};

// ---------------------------------------------------------------------------
// Copyable code block: code block with an inline copy button.
// ---------------------------------------------------------------------------

interface CopyableCodeBlockProps {
	code: string;
	label: string;
	className?: string;
}

export const CopyableCodeBlock: FC<CopyableCodeBlockProps> = ({
	code,
	label,
	className,
}) => {
	return (
		<div className="relative">
			<div className="absolute right-2 top-2 z-10">
				<CopyButton text={code} label={label} />
			</div>
			<DebugCodeBlock code={code} className={cn("pr-10", className)} />
		</div>
	);
};

// ---------------------------------------------------------------------------
// Pill toggle: compact toggle button for optional metadata sections.
// ---------------------------------------------------------------------------

interface PillToggleProps {
	label: string;
	count?: number;
	isActive: boolean;
	onToggle: () => void;
	icon?: ReactNode;
}

export const PillToggle: FC<PillToggleProps> = ({
	label,
	count,
	isActive,
	onToggle,
	icon,
}) => {
	return (
		<button
			type="button"
			aria-pressed={isActive}
			className={cn(
				"inline-flex items-center gap-1 rounded-full border-0 px-2.5 py-0.5 text-2xs font-medium transition-colors",
				isActive
					? "bg-surface-secondary text-content-primary"
					: "bg-transparent text-content-secondary hover:text-content-primary hover:bg-surface-secondary/50",
			)}
			onClick={onToggle}
		>
			{icon}
			{label}
			{count !== undefined && count > 0 ? ` (${count})` : null}
		</button>
	);
};

// ---------------------------------------------------------------------------
// Role badge: role-colored badge for message transcripts.
// ---------------------------------------------------------------------------

interface RoleBadgeProps {
	role: string;
}

export const RoleBadge: FC<RoleBadgeProps> = ({ role }) => {
	return (
		<Badge size="xs" variant={getRoleBadgeVariant(role)}>
			{role}
		</Badge>
	);
};

// ---------------------------------------------------------------------------
// Empty helper: fallback message for absent data sections.
// ---------------------------------------------------------------------------

interface EmptyHelperProps {
	message: string;
}

export const EmptyHelper: FC<EmptyHelperProps> = ({ message }) => {
	return <p className="text-sm leading-6 text-content-secondary">{message}</p>;
};

// ---------------------------------------------------------------------------
// Key-value grid: shared definition list for Options/Usage/Policy sections.
// ---------------------------------------------------------------------------

interface KeyValueGridProps {
	entries: Record<string, unknown>;
	/** Format value for display. Defaults to String(value). */
	formatValue?: (value: unknown) => string;
}

export const KeyValueGrid: FC<KeyValueGridProps> = ({
	entries,
	formatValue,
}) => {
	const fmt =
		formatValue ??
		((v: unknown) =>
			typeof v === "object" && v !== null ? safeJsonStringify(v) : String(v));

	return (
		<dl className="grid grid-cols-[auto_1fr] gap-x-3 gap-y-0.5 text-xs">
			{Object.entries(entries).map(([key, value]) => (
				<div key={key} className="contents">
					<dt className="text-content-tertiary">{key}</dt>
					<dd className="break-words font-medium text-content-primary">
						{fmt(value)}
					</dd>
				</div>
			))}
		</dl>
	);
};

// ---------------------------------------------------------------------------
// Metadata item: compact label : value pair for metadata bars.
// ---------------------------------------------------------------------------

interface MetadataItemProps {
	label: string;
	value: ReactNode;
}

export const MetadataItem: FC<MetadataItemProps> = ({ label, value }) => {
	return (
		<span className="text-xs text-content-secondary">
			<span className="text-content-tertiary">{label}:</span>{" "}
			<span className="font-medium text-content-primary">{value}</span>
		</span>
	);
};
