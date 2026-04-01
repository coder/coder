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
	DEBUG_PANEL_METADATA_CLASS_NAME,
	DebugCodeBlock,
	DebugDataSection,
} from "./DebugPanelPrimitives";
import {
	annotateRedactedJson,
	computeDurationMs,
	getStatusBadgeVariant,
	type NormalizedAttempt,
} from "./debugPanelUtils";

interface DebugAttemptAccordionProps {
	attempts: NormalizedAttempt[];
	rawFallback?: string;
}

const renderJsonBlock = (value: unknown, fallbackCopy: string) => {
	if (
		!value ||
		(typeof value === "string" && value.length === 0) ||
		(typeof value === "object" && Object.keys(value as object).length === 0)
	) {
		return (
			<p className="text-sm leading-6 text-content-secondary">{fallbackCopy}</p>
		);
	}

	if (typeof value === "string") {
		return <DebugCodeBlock code={value} />;
	}

	return (
		<DebugCodeBlock
			code={JSON.stringify(annotateRedactedJson(value), null, 2)}
		/>
	);
};

const getAttemptTimingLabel = (attempt: NormalizedAttempt): string => {
	const startedLabel = attempt.started_at
		? formatDateTime(attempt.started_at, DATE_FORMAT.TIME_24H)
		: "—";
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
		return (
			<DebugDataSection
				title="Unable to parse raw attempts"
				description="Showing the original payload exactly as it was captured."
			>
				<DebugCodeBlock code={rawFallback} />
			</DebugDataSection>
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
									<p className={DEBUG_PANEL_METADATA_CLASS_NAME}>
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
									{renderJsonBlock(
										attempt.raw_request,
										"No raw request captured.",
									)}
								</DebugDataSection>
								<DebugDataSection title="Raw response">
									{renderJsonBlock(
										attempt.raw_response,
										"No raw response captured.",
									)}
								</DebugDataSection>
								<DebugDataSection title="Error">
									{renderJsonBlock(attempt.error, "No error captured.")}
								</DebugDataSection>
							</div>
						</CollapsibleContent>
					</div>
				</Collapsible>
			))}
		</div>
	);
};
