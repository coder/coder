import { ChevronDownIcon, WrenchIcon } from "lucide-react";
import { type FC, useState } from "react";
import type { ChatDebugStep } from "#/api/typesGenerated";
import { Badge } from "#/components/Badge/Badge";
import {
	Collapsible,
	CollapsibleContent,
	CollapsibleTrigger,
} from "#/components/Collapsible/Collapsible";
import { cn } from "#/utils/cn";
import { DebugAttemptAccordion } from "./DebugAttemptAccordion";
import {
	CopyableCodeBlock,
	DEBUG_PANEL_METADATA_CLASS_NAME,
	DebugDataSection,
	EmptyHelper,
	MetadataItem,
	PillToggle,
} from "./DebugPanelPrimitives";
import {
	MessageRow,
	ToolBadge,
	ToolEventCard,
	ToolPayloadDisclosure,
} from "./DebugStepCardTooling";
import {
	coerceStepRequest,
	coerceStepResponse,
	coerceUsage,
	compactDuration,
	computeDurationMs,
	extractTokenCounts,
	formatTokenSummary,
	getStatusBadgeVariant,
	normalizeAttempts,
	safeJsonStringify,
	TRANSCRIPT_PREVIEW_COUNT,
} from "./debugPanelUtils";

interface DebugStepCardProps {
	step: ChatDebugStep;
	defaultOpen?: boolean;
}

type SectionKey = "tools" | "options" | "usage" | "policy";

export const DebugStepCard: FC<DebugStepCardProps> = ({
	step,
	defaultOpen = false,
}) => {
	// Single active metadata pill – only one section open at a time.
	const [activeSection, setActiveSection] = useState<SectionKey | null>(null);

	// Transcript preview – show last N messages by default.
	const [showAllMessages, setShowAllMessages] = useState(false);

	const toggleSection = (key: SectionKey) => {
		setActiveSection((prev) => (prev === key ? null : key));
	};

	// Coerce payloads defensively.
	const request = coerceStepRequest(step.normalized_request);
	const response = coerceStepResponse(step.normalized_response);
	const stepUsage = coerceUsage(step.usage);
	const mergedUsage =
		Object.keys(stepUsage).length > 0 ? stepUsage : response.usage;
	const tokenCounts = extractTokenCounts(mergedUsage);
	const tokenLabel = formatTokenSummary(tokenCounts.input, tokenCounts.output);
	const normalizedAttempts = normalizeAttempts(step.attempts);
	const attemptCount = normalizedAttempts.parsed.length;

	const durationMs = computeDurationMs(step.started_at, step.finished_at);
	const durationLabel = durationMs !== null ? compactDuration(durationMs) : "—";

	// Model: prefer request model, then response model.
	const model = request.model ?? response.model;

	// Counts for pill badges.
	const toolCount = request.tools.length;
	const optionCount = Object.keys(request.options).length;
	const usageEntryCount = Object.keys(mergedUsage).length;
	const policyCount = Object.keys(request.policy).length;
	const hasPills =
		toolCount > 0 || optionCount > 0 || usageEntryCount > 0 || policyCount > 0;

	// Transcript preview slicing.
	const totalMessages = request.messages.length;
	const isTruncated =
		!showAllMessages && totalMessages > TRANSCRIPT_PREVIEW_COUNT;
	const visibleMessages = isTruncated
		? request.messages.slice(-TRANSCRIPT_PREVIEW_COUNT)
		: request.messages;
	const hiddenCount = totalMessages - visibleMessages.length;

	// Detect whether there is meaningful output.
	const hasOutput =
		!!response.content ||
		response.toolCalls.length > 0 ||
		response.warnings.length > 0 ||
		!!response.finishReason;

	// Detect whether there is an error payload.
	const stringError =
		typeof step.error === "string" ? (step.error as string) : undefined;
	const hasError =
		(stringError !== undefined && stringError.trim().length > 0) ||
		(!!step.error &&
			typeof step.error === "object" &&
			Object.keys(step.error).length > 0);
	const errorCode = stringError ?? safeJsonStringify(step.error);

	return (
		<Collapsible defaultOpen={defaultOpen}>
			<div className="border-l border-l-border-default/50">
				<CollapsibleTrigger asChild>
					<button
						type="button"
						className="group flex w-full items-center gap-2 border-0 bg-transparent px-3 py-1 text-left transition-colors hover:bg-surface-secondary/20"
					>
						<div className="min-w-0 flex flex-1 items-center gap-2 overflow-hidden">
							<span className="shrink-0 text-xs font-medium text-content-tertiary">
								Step {step.step_number}
							</span>
							{model ? (
								<span className="min-w-0 truncate text-xs text-content-secondary">
									{model}
								</span>
							) : null}
							<span className="shrink-0 whitespace-nowrap text-xs text-content-tertiary">
								{durationLabel}
							</span>
							{tokenLabel ? (
								<span className="shrink-0 whitespace-nowrap text-xs text-content-tertiary">
									{tokenLabel}
								</span>
							) : null}
						</div>
						<div className="flex shrink-0 items-center gap-1.5">
							<Badge
								size="xs"
								variant={getStatusBadgeVariant(step.status)}
								className="shrink-0"
							>
								{step.status || "unknown"}
							</Badge>
							<ChevronDownIcon
								className={cn(
									"size-3.5 shrink-0 text-content-secondary transition-transform",
									"group-data-[state=open]:rotate-180",
								)}
							/>
						</div>
					</button>
				</CollapsibleTrigger>

				<CollapsibleContent className="space-y-3 px-3 pb-3">
					{/* ── Metadata bar ────────────────────────────── */}
					<div className={DEBUG_PANEL_METADATA_CLASS_NAME}>
						{model ? <MetadataItem label="Model" value={model} /> : null}
						{request.options.max_output_tokens !== undefined ||
						request.options.maxOutputTokens !== undefined ||
						request.options.max_tokens !== undefined ||
						request.options.maxTokens !== undefined ? (
							<MetadataItem
								label="Max tokens"
								value={String(
									request.options.max_output_tokens ??
										request.options.maxOutputTokens ??
										request.options.max_tokens ??
										request.options.maxTokens,
								)}
							/>
						) : null}
						{request.policy.tool_choice !== undefined ||
						request.policy.toolChoice !== undefined ? (
							<MetadataItem
								label="Tool choice"
								value={String(
									request.policy.tool_choice ?? request.policy.toolChoice,
								)}
							/>
						) : null}
						{attemptCount > 0 ? (
							<span className="text-xs text-content-tertiary">
								{attemptCount} {attemptCount === 1 ? "attempt" : "attempts"}
							</span>
						) : null}
					</div>

					{/* ── Pill toggles (single active) ───────────── */}
					{hasPills ? (
						<div className="flex flex-wrap gap-1">
							{toolCount > 0 ? (
								<PillToggle
									label="Tools"
									count={toolCount}
									isActive={activeSection === "tools"}
									onToggle={() => toggleSection("tools")}
									icon={<WrenchIcon className="size-3" />}
								/>
							) : null}
							{optionCount > 0 ? (
								<PillToggle
									label="Options"
									count={optionCount}
									isActive={activeSection === "options"}
									onToggle={() => toggleSection("options")}
								/>
							) : null}
							{usageEntryCount > 0 ? (
								<PillToggle
									label="Usage"
									count={usageEntryCount}
									isActive={activeSection === "usage"}
									onToggle={() => toggleSection("usage")}
								/>
							) : null}
							{policyCount > 0 ? (
								<PillToggle
									label="Policy"
									count={policyCount}
									isActive={activeSection === "policy"}
									onToggle={() => toggleSection("policy")}
								/>
							) : null}
						</div>
					) : null}

					{/* ── Active metadata section ────────────────── */}
					{activeSection === "tools" && toolCount > 0 ? (
						<div className="flex flex-col gap-1.5">
							{request.tools.map((tool) => (
								<div
									key={tool.name}
									className="rounded-md border border-solid border-border-default/40 bg-surface-secondary/10 p-2.5"
								>
									<ToolBadge label={tool.name} />
									{tool.description ? (
										<p className="mt-1 break-words text-2xs leading-4 text-content-secondary">
											{tool.description}
										</p>
									) : null}
									<ToolPayloadDisclosure
										label="JSON schema"
										code={tool.inputSchema}
										copyLabel={`Copy ${tool.name} JSON schema`}
									/>
								</div>
							))}
						</div>
					) : null}

					{activeSection === "options" && optionCount > 0 ? (
						<DebugDataSection title="Options">
							<dl className="grid grid-cols-[auto_1fr] gap-x-3 gap-y-0.5 text-xs">
								{Object.entries(request.options).map(([key, value]) => (
									<div key={key} className="contents">
										<dt className="text-content-tertiary">{key}</dt>
										<dd className="font-medium text-content-primary">
											{typeof value === "object"
												? safeJsonStringify(value)
												: String(value)}
										</dd>
									</div>
								))}
							</dl>
						</DebugDataSection>
					) : null}

					{activeSection === "usage" && usageEntryCount > 0 ? (
						<DebugDataSection title="Usage">
							<dl className="grid grid-cols-[auto_1fr] gap-x-3 gap-y-0.5 text-xs">
								{Object.entries(mergedUsage).map(([key, value]) => (
									<div key={key} className="contents">
										<dt className="text-content-tertiary">{key}</dt>
										<dd className="font-medium text-content-primary">
											{value.toLocaleString("en-US")}
										</dd>
									</div>
								))}
							</dl>
						</DebugDataSection>
					) : null}

					{activeSection === "policy" && policyCount > 0 ? (
						<DebugDataSection title="Policy">
							<dl className="grid grid-cols-[auto_1fr] gap-x-3 gap-y-0.5 text-xs">
								{Object.entries(request.policy).map(([key, value]) => (
									<div key={key} className="contents">
										<dt className="text-content-tertiary">{key}</dt>
										<dd className="font-medium text-content-primary">
											{typeof value === "object"
												? safeJsonStringify(value)
												: String(value)}
										</dd>
									</div>
								))}
							</dl>
						</DebugDataSection>
					) : null}

					{/* ── Input / Output two-column grid ─────────── */}
					<div className="grid gap-4 lg:grid-cols-2">
						{/* ── Input column ────────────────────────── */}
						<DebugDataSection title="Input">
							{totalMessages > 0 ? (
								<div className="space-y-2">
									{hiddenCount > 0 ? (
										<button
											type="button"
											onClick={() => setShowAllMessages(true)}
											className="border-0 bg-transparent p-0 text-2xs font-medium text-content-link transition-colors hover:underline"
										>
											Show all {totalMessages} messages
										</button>
									) : null}

									{showAllMessages &&
									totalMessages > TRANSCRIPT_PREVIEW_COUNT ? (
										<button
											type="button"
											onClick={() => setShowAllMessages(false)}
											className="border-0 bg-transparent p-0 text-2xs font-medium text-content-link transition-colors hover:underline"
										>
											Show last {TRANSCRIPT_PREVIEW_COUNT} only
										</button>
									) : null}

									{visibleMessages.map((msg, idx) => (
										<MessageRow
											key={hiddenCount + idx}
											msg={msg}
											clamp={!showAllMessages}
										/>
									))}
								</div>
							) : (
								<EmptyHelper message="No input messages captured." />
							)}
						</DebugDataSection>

						{/* ── Output column ───────────────────────── */}
						<DebugDataSection title="Output">
							{hasOutput ? (
								<div className="space-y-2">
									{/* Primary response content – visually prominent. */}
									{response.content ? (
										<p className="whitespace-pre-wrap text-sm font-medium leading-6 text-content-primary">
											{response.content}
										</p>
									) : null}

									{/* Tool calls – structured cards with arguments. */}
									{response.toolCalls.length > 0 ? (
										<div className="space-y-1.5">
											{response.toolCalls.map((tc, idx) => (
												<ToolEventCard
													key={tc.id ?? `${tc.name}-${idx}`}
													badgeLabel={tc.name}
													toolCallId={tc.id}
													payloadLabel="Arguments"
													payload={tc.arguments}
													copyLabel={`Copy ${tc.name} arguments`}
												/>
											))}
										</div>
									) : null}

									{/* Secondary metadata: finish reason + warnings. */}
									{response.finishReason ? (
										<span className="block text-2xs text-content-tertiary">
											Finish: {response.finishReason}
										</span>
									) : null}
									{response.warnings.length > 0 ? (
										<div className="space-y-0.5">
											{response.warnings.map((w, idx) => (
												<p key={idx} className="text-xs text-content-warning">
													⚠ {w}
												</p>
											))}
										</div>
									) : null}
								</div>
							) : (
								<EmptyHelper message="No output captured." />
							)}
						</DebugDataSection>
					</div>

					{/* ── Error ───────────────────────────────────── */}
					{hasError ? (
						<DebugDataSection title="Error">
							<CopyableCodeBlock
								code={errorCode}
								label={
									stringError !== undefined
										? "Copy error text"
										: "Copy error JSON"
								}
							/>
						</DebugDataSection>
					) : null}

					{/* ── Request body JSON (lower priority) ─────── */}
					<Collapsible>
						<CollapsibleTrigger asChild>
							<button
								type="button"
								className="group/raw flex items-center gap-1.5 border-0 bg-transparent p-0 text-xs font-medium text-content-secondary transition-colors hover:text-content-primary"
							>
								<ChevronDownIcon className="size-3 transition-transform group-data-[state=open]/raw:rotate-180" />
								Request body
							</button>
						</CollapsibleTrigger>
						<CollapsibleContent className="mt-1.5">
							<CopyableCodeBlock
								code={safeJsonStringify(step.normalized_request)}
								label="Copy request body JSON"
							/>
						</CollapsibleContent>
					</Collapsible>

					{/* ── Response body JSON ──────────────────────── */}
					{step.normalized_response ? (
						<Collapsible>
							<CollapsibleTrigger asChild>
								<button
									type="button"
									className="group/raw flex items-center gap-1.5 border-0 bg-transparent p-0 text-xs font-medium text-content-secondary transition-colors hover:text-content-primary"
								>
									<ChevronDownIcon className="size-3 transition-transform group-data-[state=open]/raw:rotate-180" />
									Response body
								</button>
							</CollapsibleTrigger>
							<CollapsibleContent className="mt-1.5">
								<CopyableCodeBlock
									code={safeJsonStringify(step.normalized_response)}
									label="Copy response body JSON"
								/>
							</CollapsibleContent>
						</Collapsible>
					) : null}

					{/* ── Raw HTTP attempts ───────────────────────── */}
					{attemptCount > 0 ||
					(normalizedAttempts.rawFallback &&
						normalizedAttempts.rawFallback !== "{}" &&
						normalizedAttempts.rawFallback !== "[]") ? (
						<DebugDataSection title="Raw attempts">
							<DebugAttemptAccordion
								attempts={normalizedAttempts.parsed}
								rawFallback={normalizedAttempts.rawFallback}
							/>
						</DebugDataSection>
					) : null}
				</CollapsibleContent>
			</div>
		</Collapsible>
	);
};
