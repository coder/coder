/** @jsxImportSource react */
import { defineCatalog } from "@json-render/core";
import { defineRegistry, schema } from "@json-render/react";
import { createElement, type ReactNode } from "react";
import { z } from "zod";

// ── Catalog ──────────────────────────────────────────────────────────

export const catalog = defineCatalog(schema, {
	components: {
		Card: {
			props: z.object({
				title: z.string().optional(),
				description: z.string().optional(),
			}),
			slots: ["default"],
			description: "Container with optional title and description.",
		},

		Heading: {
			props: z.object({
				text: z.string(),
				level: z.number().min(1).max(6).optional(),
			}),
			description: "Text heading (level 1-6).",
		},

		Text: {
			props: z.object({
				text: z.string(),
				emphasis: z.enum(["normal", "strong", "subtle"]).optional(),
			}),
			description: "Body text with optional emphasis.",
		},

		Badge: {
			props: z.object({
				label: z.string(),
				variant: z
					.enum(["default", "success", "warning", "error", "info"])
					.optional(),
			}),
			description: "Small label/tag with color variant.",
		},

		Metric: {
			props: z.object({
				label: z.string(),
				value: z.string(),
				change: z.string().optional(),
			}),
			description: "Key-value stat display with optional change indicator.",
		},

		ProgressBar: {
			props: z.object({
				label: z.string().optional(),
				percent: z.number().min(0).max(100),
			}),
			description: "Progress indicator with label and percentage.",
		},

		Alert: {
			props: z.object({
				message: z.string(),
				severity: z.enum(["info", "success", "warning", "error"]).optional(),
			}),
			description: "Notification banner with severity level.",
		},

		DataTable: {
			props: z.object({
				headers: z.array(z.string()),
				rows: z.array(z.array(z.string())),
			}),
			description: "Simple table with headers and rows.",
		},

		List: {
			props: z.object({
				items: z.array(z.string()),
				ordered: z.boolean().optional(),
			}),
			description: "Ordered or unordered list of items.",
		},

		Divider: {
			props: z.object({}),
			description: "Visual separator line.",
		},

		CodeBlock: {
			props: z.object({
				code: z.string(),
				language: z.string().optional(),
			}),
			description: "Code display with optional language label.",
		},

		StatusIndicator: {
			props: z.object({
				label: z.string(),
				status: z.enum(["online", "offline", "warning", "error"]),
			}),
			description: "Colored dot with label indicating status.",
		},
	},
	actions: {},
});

// ── Style lookup tables ──────────────────────────────────────────────

const BADGE_VARIANTS: Record<string, string> = {
	default: "bg-surface-tertiary text-content-primary",
	success: "bg-surface-green text-content-primary",
	warning: "bg-surface-orange text-content-primary",
	error: "bg-surface-red text-content-primary",
	info: "bg-surface-sky text-content-primary",
};

const ALERT_STYLES: Record<string, { border: string; icon: string }> = {
	info: { border: "border-border-pending", icon: "\u2139" },
	success: { border: "border-border-success", icon: "\u2713" },
	warning: { border: "border-border-warning", icon: "\u26A0" },
	error: { border: "border-border-destructive", icon: "\u2715" },
};

const STATUS_DOT: Record<string, string> = {
	online: "bg-content-success",
	offline: "bg-content-disabled",
	warning: "bg-content-warning",
	error: "bg-content-destructive",
};

const HEADING_SIZE: Record<number, string> = {
	1: "text-2xl font-bold",
	2: "text-xl font-semibold",
	3: "text-lg font-semibold",
	4: "text-base font-medium",
	5: "text-sm font-medium",
	6: "text-xs font-medium",
};

// ── Registry ─────────────────────────────────────────────────────────

export const { registry: renderRegistry } = defineRegistry(catalog, {
	components: {
		Card: ({ props, children }): ReactNode => (
			<div className="rounded-lg border border-border bg-surface-secondary p-4">
				{props.title && (
					<h3 className="mb-1 text-sm font-semibold text-content-primary">
						{props.title}
					</h3>
				)}
				{props.description && (
					<p className="mb-3 text-xs text-content-secondary">
						{props.description}
					</p>
				)}
				{children}
			</div>
		),

		Heading: ({ props }): ReactNode => {
			const lvl = Math.min(6, Math.max(1, props.level ?? 2));
			const tag = `h${lvl}` as "h1" | "h2" | "h3" | "h4" | "h5" | "h6";
			const cls = `${HEADING_SIZE[lvl] ?? HEADING_SIZE[2]} text-content-primary`;
			return createElement(tag, { className: cls }, props.text);
		},

		Text: ({ props }): ReactNode => {
			const emphasisCls: Record<string, string> = {
				normal: "text-content-primary",
				strong: "font-semibold text-content-primary",
				subtle: "text-content-secondary",
			};
			return (
				<p className={`text-sm ${emphasisCls[props.emphasis ?? "normal"]}`}>
					{props.text}
				</p>
			);
		},

		Badge: ({ props }): ReactNode => (
			<span
				className={`inline-flex items-center rounded-md px-2 py-0.5 text-xs font-medium ${
					BADGE_VARIANTS[props.variant ?? "default"]
				}`}
			>
				{props.label}
			</span>
		),

		Metric: ({ props }): ReactNode => (
			<div className="flex flex-col gap-1">
				<span className="text-xs text-content-secondary">{props.label}</span>
				<div className="flex items-baseline gap-2">
					<span className="text-xl font-semibold text-content-primary">
						{props.value}
					</span>
					{props.change && (
						<span className="text-xs text-content-secondary">
							{props.change}
						</span>
					)}
				</div>
			</div>
		),

		ProgressBar: ({ props }): ReactNode => {
			const clamped = Math.min(100, Math.max(0, props.percent));
			return (
				<div className="flex flex-col gap-1">
					{props.label && (
						<div className="flex items-center justify-between">
							<span className="text-xs text-content-secondary">
								{props.label}
							</span>
							<span className="text-xs font-medium text-content-primary">
								{Math.round(clamped)}%
							</span>
						</div>
					)}
					<div className="h-2 w-full overflow-hidden rounded-full bg-surface-tertiary">
						<div
							className="h-full rounded-full bg-content-link transition-all"
							style={{ width: `${clamped}%` }}
						/>
					</div>
				</div>
			);
		},

		Alert: ({ props }): ReactNode => {
			const style = ALERT_STYLES[props.severity ?? "info"];
			return (
				<div
					className={`flex items-start gap-2 rounded-lg border p-3 ${style.border} bg-surface-secondary`}
				>
					<span className="mt-0.5 text-sm" aria-hidden="true">
						{style.icon}
					</span>
					<span className="text-sm text-content-primary">{props.message}</span>
				</div>
			);
		},

		DataTable: ({ props }): ReactNode => (
			<div className="overflow-x-auto rounded-lg border border-border">
				<table className="w-full text-left text-sm">
					<thead>
						<tr className="border-b border-border bg-surface-tertiary">
							{props.headers.map((header: string) => (
								<th
									key={header}
									className="px-3 py-2 text-xs font-medium text-content-secondary"
								>
									{header}
								</th>
							))}
						</tr>
					</thead>
					<tbody>
						{props.rows.map((row: string[], rowIdx: number) => (
							<tr
								// Rows have a stable position within the
								// static table so index keys are acceptable.
								key={rowIdx}
								className="border-b border-border last:border-b-0"
							>
								{row.map((cell: string, cellIdx: number) => (
									<td key={cellIdx} className="px-3 py-2 text-content-primary">
										{cell}
									</td>
								))}
							</tr>
						))}
					</tbody>
				</table>
			</div>
		),

		List: ({ props }): ReactNode => {
			const Tag = props.ordered ? "ol" : "ul";
			const listStyle = props.ordered ? "list-decimal" : "list-disc";
			return (
				<Tag
					className={`flex flex-col gap-1 pl-5 text-sm text-content-primary ${listStyle}`}
				>
					{props.items.map((item: string, idx: number) => (
						<li key={idx}>{item}</li>
					))}
				</Tag>
			);
		},

		Divider: (): ReactNode => <hr className="border-border" />,

		CodeBlock: ({ props }): ReactNode => (
			<div className="overflow-hidden rounded-lg border border-border">
				{props.language && (
					<div className="border-b border-border bg-surface-secondary px-3 py-1">
						<span className="text-2xs font-medium text-content-secondary">
							{props.language}
						</span>
					</div>
				)}
				<pre className="overflow-x-auto bg-surface-tertiary p-3">
					<code className="font-mono text-xs text-content-primary">
						{props.code}
					</code>
				</pre>
			</div>
		),

		StatusIndicator: ({ props }): ReactNode => (
			<div className="flex items-center gap-2">
				<span
					className={`inline-block h-2 w-2 rounded-full ${STATUS_DOT[props.status]}`}
				/>
				<span className="text-sm text-content-primary">{props.label}</span>
			</div>
		),
	},
});
