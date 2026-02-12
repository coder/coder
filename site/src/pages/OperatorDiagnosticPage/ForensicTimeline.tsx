import type { FC } from "react";
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

export const ForensicTimeline: FC<ForensicTimelineProps> = ({ events }) => {
	if (events.length === 0) {
		return (
			<p className="text-xs text-content-secondary italic py-2">
				No timeline data
			</p>
		);
	}

	return (
		<div className="max-h-[400px] overflow-y-auto">
			<ol className="relative ml-2 list-none p-0">
				{events.map((event, i) => (
					<li
						key={`${event.timestamp}-${i}`}
						className="flex items-start gap-3 pb-2 last:pb-0"
					>
						{/* Vertical line + dot */}
						<div className="relative flex flex-col items-center">
							<div
								className={cn(
									"size-2.5 rounded-full shrink-0 mt-1.5 z-10",
									severityDotClass[event.severity],
								)}
							/>
							{i < events.length - 1 && (
								<div className="absolute top-3.5 w-px h-full bg-border" />
							)}
						</div>

						{/* Content */}
						<div className="flex items-baseline gap-3 min-w-0 pb-1">
							<span className="font-mono text-2xs text-content-secondary shrink-0">
								{formatTime(event.timestamp)}
							</span>
							<span className="text-xs text-content-primary">
								{event.description}
							</span>
						</div>
					</li>
				))}
			</ol>
		</div>
	);
};
