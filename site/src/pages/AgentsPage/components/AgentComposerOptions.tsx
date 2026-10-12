import { cn } from "cn";
import type React from "react";
import { Skeleton } from "#/components/Skeleton/Skeleton";
import {
	ModelSelector,
	type ModelSelectorOption,
} from "#/modules/aiModels/ModelSelector";
import { ProviderIcon } from "#/modules/aiModels/ProviderIcon";
import { useAgentComposer } from "./AgentComposer/context";
import {
	AgentComposerBadges,
	AgentComposerPlanningBadge,
	composerPillSizingClasses,
} from "./AgentComposerBadges";
import {
	type AgentComposerOptionsData,
	OptionsContext,
	useAgentComposerOptions,
} from "./AgentComposerOptionsContext";
import { AgentComposerOptionsMenu } from "./AgentComposerOptionsMenu";

/** Shares controlled tool settings with the menu and badges. */
const AgentComposerOptionsProvider = ({
	children,
	...data
}: AgentComposerOptionsData & { children: React.ReactNode }) => (
	<OptionsContext value={data}>{children}</OptionsContext>
);

const AgentComposerOptionsFrame = ({
	children,
}: {
	children: React.ReactNode;
}) => (
	// flex-1 routes free row space to the growing pills.
	<div className="flex min-w-0 flex-1 items-center gap-1">{children}</div>
);

/** Model catalog selection and reasoning controls, independent of tool state. */
type AgentComposerModelProps = {
	selectedModel: string;
	onModelChange: (value: string) => void;
	modelOptions: readonly ModelSelectorOption[];
	modelSelectorPlaceholder: string;
	reasoningEffort?: string;
	onReasoningEffortChange?: (value: string) => void;
	isModelCatalogLoading: boolean;
};

const AgentComposerModel = (props: AgentComposerModelProps) => {
	const { state } = useAgentComposer();
	const { isViewOnly } = useAgentComposerOptions();

	if (props.isModelCatalogLoading) {
		return <Skeleton className="h-6 w-24 rounded" />;
	}

	if (isViewOnly) {
		const model = props.modelOptions.find(
			(option) => option.id === props.selectedModel,
		);

		return (
			<span className="inline-flex min-w-0 shrink items-center gap-1 rounded-full bg-surface-secondary px-2 py-0.5 text-xs font-medium text-content-secondary">
				{model && (
					<ProviderIcon
						provider={model.provider}
						icon={model.providerIcon}
						className="size-3 shrink-0"
					/>
				)}
				<span className="truncate">
					{model?.displayName ?? props.modelSelectorPlaceholder}
				</span>
			</span>
		);
	}

	return (
		<ModelSelector
			value={props.selectedModel}
			onValueChange={props.onModelChange}
			options={props.modelOptions}
			disabled={state.isDisabled}
			placeholder={props.modelSelectorPlaceholder}
			className={cn(composerPillSizingClasses, "md:h-auto")}
			dropdownSide="top"
			dropdownAlign="start"
			enableMobileFullWidthDropdown
			reasoningEffort={props.reasoningEffort}
			onReasoningEffortChange={props.onReasoningEffortChange}
		/>
	);
};

/** Composer controls that callers can arrange or omit independently. */
export const AgentComposerOptions = {
	Provider: AgentComposerOptionsProvider,
	Frame: AgentComposerOptionsFrame,
	Model: AgentComposerModel,
	Menu: AgentComposerOptionsMenu,
	PlanningBadge: AgentComposerPlanningBadge,
	Badges: AgentComposerBadges,
};
