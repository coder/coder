import { ChevronDownIcon, ChevronRightIcon } from "lucide-react";
import { type FC, useState } from "react";
import { cn } from "utils/cn";
import type { DiagnosticTimelineEvent } from "./types";

interface ForensicTimelineProps {
	events: DiagnosticTimelineEvent[];
}

const severityDotClass: Record<DiagnosticTimelineEvent["severity"], string> = {
	info: "bg-highlight-sky",
	warning: "bg-content-warning",
	error: "bg-content-destructive",
};

function formatTime(iso: string): string {
	const d = new Date(iso);
	return d.toLocaleTimeString([], {
		hour: "2-digit",
		minute: "2-digit",
		second: "2-digit",
		hour12: false,
	});
}

type EventGroup =
	| { type: "single"; event: DiagnosticTimelineEvent }
	| { type: "collapsed"; events: DiagnosticTimelineEvent[]; kind: string };

function groupConsecutiveEvents(
	events: DiagnosticTimelineEvent[],
): EventGroup[] {
	const groups: EventGroup[] = [];
	let i = 0;
	while (i < events.length) {
		let j = i + 1;
		while (j < events.length && events[j].kind === events[i].kind) {
			j++;
		}
		const run = events.slice(i, j);
		if (run.length >= 3) {
			groups.push({ type: "collapsed", events: run, kind: run[0].kind });
		} else {
			for (const e of run) {
				groups.push({ type: "single", event: e });
			}
		}
		i = j;
	}
	return groups;
}

const SingleEventItem: FC<{
	event: DiagnosticTimelineEvent;
	isLast: boolean;
}> = ({ event, isLast }) => (
	<li className={cn("flex items-start gap-3 pb-2", isLast && "pb-0")}>
		<div className="relative flex flex-col items-center">
			<div
				className={cn(
					"size-2.5 rounded-full shrink-0 mt-1.5 z-10",
					severityDotClass[event.severity],
				)}
			/>
			{!isLast && <div className="absolute top-3.5 w-px h-full bg-border" />}
		</div>
		<div className="flex items-baseline gap-3 min-w-0 pb-1">
			<span className="font-mono text-2xs text-content-secondary shrink-0">
				{formatTime(event.timestamp)}
			</span>
			<span className={cn(
				"text-xs",
				event.description.toLowerCase().startsWith("system")
					? "text-content-secondary italic"
					: "text-content-primary",
			)}>
				{event.description}
			</span>
		</div>
	</li>
);

const CollapsedGroupItem: FC<{
	events: DiagnosticTimelineEvent[];
	expanded: boolean;
	onToggle: () => void;
	isLast: boolean;
}> = ({ events, expanded, onToggle, isLast }) => {
	const ChevronIcon = expanded ? ChevronDownIcon : ChevronRightIcon;

	if (expanded) {
		return (
			<>
				<li className="flex items-start gap-3 pb-2">
					<div className="relative flex flex-col items-center">
						<div
							className={cn(
								"size-2.5 rounded-full shrink-0 mt-1.5 z-10",
								severityDotClass[events[0].severity],
							)}
						/>
						<div className="absolute top-3.5 w-px h-full bg-border" />
					</div>
					<button
						type="button"
						className="flex items-center gap-3 min-w-0 pb-1 cursor-pointer hover:text-content-primary text-content-secondary text-xs bg-transparent border-none p-0"
						onClick={onToggle}
					>
						<span className="font-mono text-2xs shrink-0">
							{formatTime(events[0].timestamp)} -{" "}
							{formatTime(events[events.length - 1].timestamp)}
						</span>
						<span>
							{events.length}&times; {events[0].description}
						</span>
						<ChevronIcon className="size-3 shrink-0" />
					</button>
				</li>
				{events.map((event, i) => (
					<SingleEventItem
						key={`${event.timestamp}-${i}`}
						event={event}
						isLast={isLast && i === events.length - 1}
					/>
				))}
			</>
		);
	}

	return (
		<li className={cn("flex items-start gap-3 pb-2", isLast && "pb-0")}>
			<div className="relative flex flex-col items-center">
				<div
					className={cn(
						"size-2.5 rounded-full shrink-0 mt-1.5 z-10",
						severityDotClass[events[0].severity],
					)}
				/>
				{!isLast && <div className="absolute top-3.5 w-px h-full bg-border" />}
			</div>
			<button
				type="button"
				className="flex items-center gap-3 min-w-0 pb-1 cursor-pointer hover:text-content-primary text-content-secondary text-xs bg-transparent border-none p-0"
				onClick={onToggle}
			>
				<span className="font-mono text-2xs shrink-0">
					{formatTime(events[0].timestamp)} -{" "}
					{formatTime(events[events.length - 1].timestamp)}
				</span>
				<span>
					{events.length}&times; {events[0].description}
				</span>
				<ChevronIcon className="size-3 shrink-0" />
			</button>
		</li>
	);
};

export const ForensicTimeline: FC<ForensicTimelineProps> = ({ events }) => {
	const [expandedGroups, setExpandedGroups] = useState<Set<number>>(
		() => new Set(),
	);

	if (events.length === 0) {
		return (
			<p className="text-xs text-content-secondary italic py-2">
				No timeline data
			</p>
		);
	}

	const groups = groupConsecutiveEvents(events);

	const toggleGroup = (index: number) => {
		setExpandedGroups((prev) => {
			const next = new Set(prev);
			if (next.has(index)) {
				next.delete(index);
			} else {
				next.add(index);
			}
			return next;
		});
	};

	return (
		<div className="max-h-[400px] overflow-y-auto">
			<ol className="relative ml-2 list-none p-0">
				{groups.map((group, groupIndex) => {
					const isLast = groupIndex === groups.length - 1;
					if (group.type === "single") {
						return (
							<SingleEventItem
								key={`single-${group.event.timestamp}-${groupIndex}`}
								event={group.event}
								isLast={isLast}
							/>
						);
					}
					return (
						<CollapsedGroupItem
							key={`group-${group.events[0].timestamp}-${groupIndex}`}
							events={group.events}
							expanded={expandedGroups.has(groupIndex)}
							onToggle={() => toggleGroup(groupIndex)}
							isLast={isLast}
						/>
					);
				})}
			</ol>
		</div>
	);
};
