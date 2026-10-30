import { cn } from "cn";
import { useCallback, useLayoutEffect, useRef } from "react";
import { VariableSizeList as List } from "react-window";
import type { WorkspaceAgentLogSource } from "#/api/typesGenerated";
import { Badge } from "#/components/Badge/Badge";
import { ExternalImage } from "#/components/ExternalImage/ExternalImage";
import type { Line } from "#/components/Logs/LogLine";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { AGENT_LOG_LINE_HEIGHT, AgentLogLine } from "./AgentLogLine";

// Fallback log used in places where we must always have a valid log source.
// We need this to support deployments that were made before `coder_script` was
// created and that haven't restarted their agents yet
const fallbackLog: WorkspaceAgentLogSource = {
	created_at: "",
	display_name: "Logs",
	icon: "",
	id: "00000000-0000-0000-0000-000000000000",
	workspace_agent_id: "",
};

type AgentLogsProps = Omit<
	React.ComponentProps<typeof List>,
	"children" | "itemSize" | "itemCount" | "itemKey"
> & {
	logs: readonly Line[];
	sources: readonly WorkspaceAgentLogSource[];
	overflowed: boolean;
	showSourceIcons?: boolean;
	ref?: React.Ref<List>;
};

export const AgentLogs: React.FC<AgentLogsProps> = ({
	logs,
	sources,
	overflowed,
	className,
	showSourceIcons = true,
	ref,
	...listProps
}) => {
	const logSourceById = Object.fromEntries(sources.map((s) => [s.id, s]));
	const getLogSource = (id: string) => logSourceById[id] || fallbackLog;

	const listRef = useRef<List>(null);
	const mergeListRef = useCallback(
		(instance: List | null) => {
			listRef.current = instance;
			if (typeof ref === "function") {
				ref(instance);
			} else if (ref) {
				ref.current = instance;
			}
		},
		[ref],
	);

	// A log line's real height depends on its content (long lines wrap, so a
	// single log entry can span multiple visual rows). A fixed itemSize makes
	// react-window compute the total scroll height as itemCount * itemSize,
	// which drifts further from reality with every taller-than-expected row.
	// With enough rows the estimated height falls short of the real content
	// and the list can no longer scroll to the last line (coder/coder#25692).
	// Measuring each row and feeding the true height back keeps the total
	// height accurate regardless of how many rows there are.
	const sizeMapRef = useRef<Map<number, number>>(new Map());
	const pendingResetIndexRef = useRef<number | null>(null);
	// resetAfterIndex tells react-window to recompute offsets from the first
	// row whose height changed. The list ref is assigned after the child rows
	// commit, so on the initial mount we can't reset from within a row's
	// measurement effect; instead we record the lowest changed index and flush
	// it from a parent layout effect once the ref is available.
	const flushPendingReset = useCallback(() => {
		const index = pendingResetIndexRef.current;
		if (index !== null && listRef.current) {
			pendingResetIndexRef.current = null;
			listRef.current.resetAfterIndex(index);
		}
	}, []);
	const setRowHeight = useCallback(
		(index: number, height: number) => {
			if (sizeMapRef.current.get(index) === height) {
				return;
			}
			sizeMapRef.current.set(index, height);
			pendingResetIndexRef.current =
				pendingResetIndexRef.current === null
					? index
					: Math.min(pendingResetIndexRef.current, index);
			flushPendingReset();
		},
		[flushPendingReset],
	);
	const getRowHeight = useCallback(
		(index: number) => sizeMapRef.current.get(index) ?? AGENT_LOG_LINE_HEIGHT,
		[],
	);

	// Flush measurements taken before the list ref existed (initial mount).
	useLayoutEffect(() => {
		flushPendingReset();
	});

	return (
		<div className="bg-surface-secondary relative">
			<List
				{...listProps}
				ref={mergeListRef}
				itemCount={logs.length}
				itemSize={getRowHeight}
				estimatedItemSize={AGENT_LOG_LINE_HEIGHT}
				itemKey={(index) => logs[index]?.id || index}
				// We need the div selector to be able to apply the padding
				// top from startupLogs
				className={cn(
					"py-4 [&>div]:relative bg-surface-secondary",
					// Add extra padding so that overflow indicator can't
					// fully cover up lines of text
					overflowed && "pb-10",
					className,
				)}
			>
				{({ index, style }) => {
					const log = logs[index];
					const logSource = getLogSource(log.sourceId);

					const doesNextLineHaveDifferentSource =
						index < logs.length - 1 &&
						getLogSource(logs[index + 1].sourceId).id !== log.sourceId;

					// We don't want every line to repeat the icon, because
					// that is ugly and repetitive. This removes the icon
					// for subsequent lines of the same source and shows a
					// line instead, visually indicating they are from the
					// same source.
					const shouldHideSource =
						index > 0 &&
						getLogSource(logs[index - 1].sourceId).id === log.sourceId;

					return (
						<MeasuredLogRow
							index={index}
							style={style}
							onMeasure={setRowHeight}
						>
							<AgentLogLine
								line={log}
								sourceIcon={
									showSourceIcons ? (
										shouldHideSource ? (
											<SourceContinuationLine
												isLastInRun={doesNextLineHaveDifferentSource}
											/>
										) : (
											<SourceIcon source={logSource} />
										)
									) : null
								}
							/>
						</MeasuredLogRow>
					);
				}}
			</List>

			{overflowed && (
				<Tooltip>
					<TooltipTrigger asChild>
						<Badge
							asChild
							className="max-w-fit absolute bottom-3 left-1/2 -translate-x-1/2"
						>
							<span>Logs overflowed</span>
						</Badge>
					</TooltipTrigger>
					<TooltipContent
						asChild
						className="w-full text-sm text-content-secondary bg-surface-primary max-w-prose leading-relaxed m-0 p-4"
					>
						<p>
							Startup logs exceeded the max size of{" "}
							<span className="tracking-wide font-mono">1MB</span>, and will not
							continue to be written to the database. Logs will continue to be
							written to the{" "}
							<span className="font-mono bg-surface-tertiary rounded-md px-1.5 py-0.5">
								/tmp/coder-startup-script.log
							</span>{" "}
							file in the workspace.
						</p>
					</TooltipContent>
				</Tooltip>
			)}
		</div>
	);
};

type MeasuredLogRowProps = {
	index: number;
	// react-window's positioning style for the row (absolute top/left/width).
	style: React.CSSProperties;
	onMeasure: (index: number, height: number) => void;
	children: React.ReactNode;
};

// Wraps a log line and reports its rendered height back to the virtualized
// list. The height is left to the content (`height: auto`) so wrapped or
// multi-line output is measured accurately instead of being assumed to be a
// single fixed-height row.
const MeasuredLogRow: React.FC<MeasuredLogRowProps> = ({
	index,
	style,
	onMeasure,
	children,
}) => {
	const rowRef = useRef<HTMLDivElement>(null);

	useLayoutEffect(() => {
		const el = rowRef.current;
		if (!el) {
			return;
		}
		const report = () => {
			const height = el.getBoundingClientRect().height;
			if (height > 0) {
				onMeasure(index, height);
			}
		};
		report();
		const observer = new ResizeObserver(report);
		observer.observe(el);
		return () => observer.disconnect();
	}, [index, onMeasure]);

	return (
		<div ref={rowRef} style={{ ...style, height: "auto" }}>
			{children}
		</div>
	);
};

type SourceIconProps = {
	source: WorkspaceAgentLogSource;
};

// Marks the first line of a run of lines from the same source. It is the run's
// only tooltip trigger, so long logs don't add a tab stop per line.
const SourceIcon: React.FC<SourceIconProps> = ({ source }) => {
	return (
		<Tooltip>
			<TooltipTrigger asChild>
				<button
					type="button"
					aria-label={source.display_name}
					className="mr-2 flex shrink-0 items-center rounded-sm focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-content-link"
				>
					{/* If no icon is specified, we show a deterministic
					colored circle to identify unique scripts. */}
					{source.icon ? (
						<ExternalImage src={source.icon} alt="" className="size-3.5" />
					) : (
						<div
							role="presentation"
							className="size-3.5 rounded-full"
							style={{
								background: determineScriptDisplayColor(source.display_name),
							}}
						/>
					)}
				</button>
			</TooltipTrigger>
			<TooltipContent side="bottom">
				{source.display_name}
				{!source.icon && (
					<i>
						<br />
						No icon specified!
					</i>
				)}
			</TooltipContent>
		</Tooltip>
	);
};

type SourceContinuationLineProps = {
	isLastInRun: boolean;
};

// Decorative connector shown instead of the icon on the remaining lines of a
// run, visually tying them to the run's first line.
const SourceContinuationLine: React.FC<SourceContinuationLineProps> = ({
	isLastInRun,
}) => {
	return (
		<div
			aria-hidden
			className="size-3.5 mr-2 flex justify-center relative shrink-0"
		>
			<div
				// dashed-line class comes from AgentLogLine component
				className={cn(
					"dashed-line w-0.5 rounded-xs bg-surface-tertiary h-full",
					isLastInRun && "h-1/2",
				)}
			/>
			{isLastInRun && (
				<div className="dashed-line h-0.5 w-1/2 top-[calc(50%-2px)] left-[calc(50%-1px)] rounded-xs absolute bg-surface-tertiary" />
			)}
		</div>
	);
};

// These colors were picked at random. Feel free to add more, adjust, or change!
// Users will not depend on these colors.
const scriptDisplayColors: readonly string[] = [
	"#85A3B2",
	"#A37EB2",
	"#C29FDE",
	"#90B3D7",
	"#829AC7",
	"#728B8E",
	"#506080",
	"#5654B0",
	"#6B56D6",
	"#7847CC",
];

const determineScriptDisplayColor = (displayName: string): string => {
	const hash = displayName.split("").reduce((hash, char) => {
		return (hash << 5) + hash + char.charCodeAt(0); // bit-shift and add for our simple hash
	}, 0);
	return scriptDisplayColors[Math.abs(hash) % scriptDisplayColors.length];
};
