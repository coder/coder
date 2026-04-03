import { ChevronDownIcon } from "lucide-react";
import { type FC, useState } from "react";
import { useQuery } from "react-query";
import { getErrorMessage } from "#/api/errors";
import { chatDebugRun } from "#/api/queries/chats";
import type { ChatDebugRunSummary } from "#/api/typesGenerated";
import { Alert } from "#/components/Alert/Alert";
import { Badge } from "#/components/Badge/Badge";
import {
	Collapsible,
	CollapsibleContent,
	CollapsibleTrigger,
} from "#/components/Collapsible/Collapsible";
import { Spinner } from "#/components/Spinner/Spinner";
import { cn } from "#/utils/cn";
import { DebugStepCard } from "./DebugStepCard";
import {
	clampContent,
	coerceRunSummary,
	compactDuration,
	computeDurationMs,
	formatTokenSummary,
	getRunKindLabel,
	getStatusBadgeVariant,
	isActiveStatus,
} from "./debugPanelUtils";

interface DebugRunCardProps {
	run: ChatDebugRunSummary;
	chatId: string;
	enabled?: boolean;
}

const getDurationLabel = (startedAt: string, finishedAt?: string): string => {
	const durationMs = computeDurationMs(startedAt, finishedAt);
	return durationMs !== null ? compactDuration(durationMs) : "—";
};

export const DebugRunCard: FC<DebugRunCardProps> = ({
	run,
	chatId,
	enabled = true,
}) => {
	const [isExpanded, setIsExpanded] = useState(false);
	const runDetailQuery = useQuery({
		...chatDebugRun(chatId, run.id),
		enabled: enabled && isExpanded,
	});

	const steps = runDetailQuery.data?.steps ?? [];

	// Coerce summary from detail (preferred) → props → empty.
	const summaryVm = coerceRunSummary(
		runDetailQuery.data?.summary ?? run.summary,
	);
	const modelLabel = summaryVm.model?.trim() || run.model?.trim() || "";

	// Primary label fallback chain: firstMessage → kind.
	const primaryLabel = clampContent(
		summaryVm.primaryLabel.trim() || getRunKindLabel(run.kind),
		80,
	);

	// Token summary for the header.
	const tokenLabel = formatTokenSummary(
		summaryVm.totalInputTokens,
		summaryVm.totalOutputTokens,
	);

	// Step count from detail or summary.
	const stepCount = steps.length > 0 ? steps.length : summaryVm.stepCount;
	const durationLabel = getDurationLabel(run.started_at, run.finished_at);
	const metadataItems = [
		modelLabel || undefined,
		stepCount !== undefined && stepCount > 0
			? `${stepCount} ${stepCount === 1 ? "step" : "steps"}`
			: undefined,
		durationLabel,
		tokenLabel || undefined,
	].filter((item): item is string => item !== undefined);
	const running = isActiveStatus(run.status);

	return (
		<Collapsible open={isExpanded} onOpenChange={setIsExpanded}>
			<div>
				<CollapsibleTrigger asChild>
					<button
						type="button"
						className="group flex w-full items-center gap-2 border-0 bg-transparent px-3 py-1.5 text-left transition-colors hover:bg-surface-secondary/20"
					>
						<div className="min-w-0 flex flex-1 items-center gap-2.5 overflow-hidden">
							<p className="min-w-0 flex-1 truncate text-sm font-semibold text-content-primary">
								{primaryLabel}
							</p>
							<div className="flex shrink-0 items-center gap-2 text-xs leading-5 text-content-secondary">
								{metadataItems.map((item, index) => (
									<span
										key={`${item}-${index}`}
										className="shrink-0 whitespace-nowrap"
									>
										{item}
									</span>
								))}
							</div>
						</div>
						<div className="flex shrink-0 items-center gap-1.5">
							{running ? <Spinner size="sm" loading /> : null}
							<Badge
								size="sm"
								variant={getStatusBadgeVariant(run.status)}
								className="shrink-0"
							>
								{run.status || "unknown"}
							</Badge>
							<ChevronDownIcon
								className={cn(
									"size-4 shrink-0 text-content-secondary transition-transform",
									"group-data-[state=open]:rotate-180",
								)}
							/>
						</div>
					</button>
				</CollapsibleTrigger>
				<CollapsibleContent className="px-4 pb-4 pt-2">
					{runDetailQuery.isLoading ? (
						<div className="flex items-center gap-2 text-sm text-content-secondary">
							<Spinner size="sm" loading />
							Loading run details...
						</div>
					) : runDetailQuery.isError ? (
						<Alert severity="error" prominent>
							<p className="text-sm text-content-primary">
								{getErrorMessage(runDetailQuery.error, "Unable to load debug run details.")}
							</p>
						</Alert>
					) : (
						<div className="space-y-0">
							{steps.map((step) => (
								<DebugStepCard key={step.id} step={step} defaultOpen={false} />
							))}
							{steps.length === 0 ? (
								<p className="text-sm text-content-secondary">
									No steps recorded.
								</p>
							) : null}
						</div>
					)}
				</CollapsibleContent>
			</div>
		</Collapsible>
	);
};
