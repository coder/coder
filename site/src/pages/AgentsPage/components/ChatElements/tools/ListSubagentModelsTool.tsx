import type React from "react";
import { ScrollArea } from "#/components/ScrollArea/ScrollArea";
import { ProviderIcon } from "#/pages/AISettingsPage/ProvidersPage/components/ProviderIcon";
import { formatProviderLabel } from "#/utils/aiProviders";
import { formatContextLimit } from "../ModelSelector";
import { ToolCall } from "./ToolCall";
import { asNumber, asRecord, asString, type ToolStatus } from "./utils";

const ModelRow: React.FC<{ model: unknown }> = ({ model }) => {
	const rec = asRecord(model);
	if (!rec) {
		return null;
	}
	const displayName = asString(rec.display_name).trim();
	const modelName = asString(rec.model).trim();
	const contextLimit = asNumber(rec.context_limit);
	const context =
		contextLimit !== undefined && contextLimit > 0
			? formatContextLimit(contextLimit)
			: "";
	const efforts = Array.isArray(rec.reasoning_efforts)
		? rec.reasoning_efforts.map(asString).filter((effort) => effort.trim())
		: [];
	const effortRange =
		efforts.length > 1
			? `${efforts[0]} - ${efforts[efforts.length - 1]}`
			: (efforts[0] ?? "");
	const title = displayName || modelName || "Unknown model";

	return (
		<div className="grid grid-cols-[minmax(0,1fr)_96px_56px] items-center gap-x-2 px-2 py-1">
			<span className="min-w-0 truncate text-xs font-medium leading-[18px] text-content-primary">
				{title}
			</span>
			{effortRange ? (
				<span className="shrink-0 text-right text-xs font-normal leading-[18px] tabular-nums text-content-secondary">
					{effortRange}
				</span>
			) : (
				<span aria-hidden="true" />
			)}
			{context ? (
				<span className="shrink-0 text-right text-xs font-normal leading-[18px] tabular-nums text-content-secondary">
					({context})
				</span>
			) : (
				<span aria-hidden="true" />
			)}
		</div>
	);
};

const ListSubagentModelsContent: React.FC<{ models: unknown[] }> = ({
	models,
}) => {
	// Group by provider like the model picker so models from one provider
	// sit under a single headed section instead of a flat list.
	const grouped = new Map<string, unknown[]>();
	for (const model of models) {
		const rec = asRecord(model);
		if (!rec) {
			continue;
		}
		const provider = asString(rec.provider).trim() || "unknown";
		const group = grouped.get(provider);
		if (group) {
			group.push(model);
			continue;
		}
		grouped.set(provider, [model]);
	}

	return (
		<ScrollArea
			className="mt-1.5 rounded-md border border-solid border-border-default"
			viewportClassName="max-h-64"
			viewportTabIndex={0}
			viewportAriaLabel="Available models"
			scrollBarClassName="w-1.5"
		>
			<div className="px-1 py-1">
				{Array.from(grouped.entries(), ([provider, providerModels], index) => (
					<div
						key={provider}
						className={
							index > 0
								? "border-0 border-t border-solid border-border-default"
								: undefined
						}
					>
						<div className="flex items-center gap-1.5 px-2 py-1">
							<ProviderIcon provider={provider} className="size-3.5" />
							<span className="text-xs font-semibold leading-[18px] text-content-secondary">
								{formatProviderLabel(provider)}
							</span>
						</div>
						{providerModels.map((model, modelIndex) => {
							const rec = asRecord(model);
							const configID = rec ? asString(rec.model_config_id).trim() : "";
							return <ModelRow key={configID || modelIndex} model={model} />;
						})}
					</div>
				))}
			</div>
		</ScrollArea>
	);
};

/**
 * Collapsed-by-default rendering for `list_subagent_models` tool calls.
 * Shows "Listed N subagent models" with a chevron; expanding reveals the
 * models accepted by spawn_agent's model_config_id argument, grouped by
 * provider like the chat model picker.
 */
export const ListSubagentModelsTool: React.FC<{
	models: unknown[];
	status: ToolStatus;
	isError: boolean;
	errorMessage?: string;
}> = ({ models, status, isError, errorMessage }) => {
	const hasContent = models.length > 0;
	const isRunning = status === "running";

	const label = isRunning
		? "Listing subagent models…"
		: `Listed ${models.length} subagent ${models.length === 1 ? "model" : "models"}`;

	return (
		<ToolCall.Root
			className="max-w-sm"
			status={status}
			isError={isError}
			errorMessage={errorMessage || "Failed to list subagent models"}
			hasContent={hasContent}
		>
			<ToolCall.Header iconName="list_subagent_models" label={label} />
			<ToolCall.Content>
				<ListSubagentModelsContent models={models} />
			</ToolCall.Content>
		</ToolCall.Root>
	);
};
