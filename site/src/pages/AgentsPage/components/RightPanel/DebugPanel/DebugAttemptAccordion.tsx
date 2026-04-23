import { ChevronDownIcon } from "lucide-react";
import type { FC } from "react";
import { Badge } from "#/components/Badge/Badge";
import {
	Collapsible,
	CollapsibleContent,
	CollapsibleTrigger,
} from "#/components/Collapsible/Collapsible";
import { cn } from "#/utils/cn";
import { DATE_FORMAT, formatDateTime, humanDuration } from "#/utils/time";
import {
	DebugCodeBlock,
	DebugDataSection,
	EmptyHelper,
} from "./DebugPanelPrimitives";
import {
	computeDurationMs,
	getStatusBadgeVariant,
	type NormalizedAttempt,
	safeJsonStringify,
} from "./debugPanelUtils";

interface DebugAttemptAccordionProps {
	attempts: NormalizedAttempt[];
	rawFallback?: string;
}

interface JsonBlockProps {
	value: unknown;
	fallbackCopy: string;
}

const JsonBlock: FC<JsonBlockProps> = ({ value, fallbackCopy }) => {
	if (
		value === null ||
		value === undefined ||
		(typeof value === "string" && value.length === 0) ||
		(typeof value === "object" && Object.keys(value as object).length === 0)
	) {
		return <EmptyHelper message={fallbackCopy} />;
	}

	if (typeof value === "string") {
		return <DebugCodeBlock code={value} />;
	}

	return <DebugCodeBlock code={safeJsonStringify(value)} />;
};

const getAttemptTimingLabel = (attempt: NormalizedAttempt): string => {
	const startedLabel = attempt.started_at
		? formatDateTime(attempt.started_at, DATE_FORMAT.TIME_24H)
		: "-";
	const finishedLabel = attempt.finished_at
		? formatDateTime(attempt.finished_at, DATE_FORMAT.TIME_24H)
		: "in progress";

	const durationMs =
		attempt.duration_ms ??
		(attempt.started_at
			? computeDurationMs(attempt.started_at, attempt.finished_at)
			: null);
	const durationLabel =
		durationMs !== null ? humanDuration(durationMs) : "Duration unavailable";

	return `${startedLabel} → ${finishedLabel} • ${durationLabel}`;
};

export const DebugAttemptAccordion: FC<DebugAttemptAccordionProps> = ({
	attempts,
	rawFallback,
}) => {
	if (rawFallback) {
		// No DebugDataSection wrapper here. The parent already
		// wraps us in <DebugDataSection title="Raw attempts">.
		return (
			<div className="flex flex-col gap-1.5">
				<p className="text-xs text-content-secondary">
					Unable to parse raw attempts. Showing the original payload exactly as
					it was captured.
				</p>
				<DebugCodeBlock code={rawFallback} />
			</div>
		);
	}

	if (attempts.length === 0) {
		return (
			<p className="text-sm text-content-secondary">No attempts captured.</p>
		);
	}

	return (
		<div className="space-y-3">
			{attempts.map((attempt, index) => (
				<Collapsible
					key={`${attempt.attempt_number}-${attempt.started_at ?? index}`}
					defaultOpen={false}
				>
					<div className="border-l border-l-border-default/50">
						<CollapsibleTrigger asChild>
							<button
								type="button"
								className="group flex w-full items-start gap-3 border-0 bg-transparent px-4 py-3 text-left transition-colors hover:bg-surface-secondary/20"
							>
								<div className="min-w-0 flex-1 space-y-2">
									<div className="flex flex-wrap items-center gap-2">
										<span className="text-sm font-semibold text-content-primary">
											Attempt {attempt.attempt_number}
										</span>
										{attempt.method || attempt.path ? (
											<span className="truncate font-mono text-xs font-medium text-content-secondary">
												{[attempt.method, attempt.path]
													.filter(Boolean)
													.join(" ")}
											</span>
										) : null}
										{attempt.response_status ? (
											<Badge
												size="xs"
												variant={
													attempt.response_status < 400
														? "green"
														: "destructive"
												}
											>
												{attempt.response_status}
											</Badge>
										) : null}
										<Badge
											size="sm"
											variant={getStatusBadgeVariant(attempt.status)}
											className="shrink-0 sm:hidden"
										>
											{attempt.status || "unknown"}
										</Badge>
									</div>
									<p className="flex flex-wrap gap-x-3 gap-y-1 text-xs leading-5 text-content-secondary">
										<span>{getAttemptTimingLabel(attempt)}</span>
									</p>
								</div>
								<div className="flex shrink-0 items-center gap-2">
									<Badge
										size="sm"
										variant={getStatusBadgeVariant(attempt.status)}
										className="hidden shrink-0 sm:inline-flex"
									>
										{attempt.status || "unknown"}
									</Badge>
									<ChevronDownIcon
										className={cn(
											"mt-0.5 size-4 shrink-0 text-content-secondary transition-transform",
											"group-data-[state=open]:rotate-180",
										)}
									/>
								</div>
							</button>
						</CollapsibleTrigger>
						<CollapsibleContent className="px-4 pb-4 pt-2">
							<div className="space-y-3">
								<DebugDataSection title="Raw request">
									<JsonBlock
										value={attempt.raw_request}
										fallbackCopy="No raw request captured."
									/>
								</DebugDataSection>
								<DebugDataSection title="Raw response">
									<JsonBlock
										value={attempt.raw_response}
										fallbackCopy="No raw response captured."
									/>
								</DebugDataSection>
								<DebugDataSection title="Error">
									<JsonBlock
										value={attempt.error}
										fallbackCopy="No error captured."
									/>
								</DebugDataSection>
							</div>
						</CollapsibleContent>
					</div>
				</Collapsible>
			))}
		</div>
	);
};
