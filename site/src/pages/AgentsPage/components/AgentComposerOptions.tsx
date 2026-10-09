import { cn } from "cn";
import type React from "react";
import { Skeleton } from "#/components/Skeleton/Skeleton";
import {
	ModelSelector,
	type ModelSelectorOption,
} from "#/modules/aiModels/ModelSelector";
import { useAgentComposer } from "./AgentComposer";
import {
	AgentComposerBadges,
	AgentComposerPlanningBadge,
	composerPillSizingClasses,
} from "./AgentComposerBadges";
import {
	type AgentComposerOptionsData,
	OptionsContext,
} from "./AgentComposerOptionsContext";
import { AgentComposerOptionsMenu } from "./AgentComposerOptionsMenu";

/** Shares controlled tool settings with the menu and badges. */
const AgentComposerOptionsProvider = ({
	children,
	...value
}: AgentComposerOptionsData & { children: React.ReactNode }) => (
	<OptionsContext value={value}>{children}</OptionsContext>
);

const AgentComposerOptionsFrame = ({
	children,
}: {
	children: React.ReactNode;
}) => <div className="flex min-w-0 flex-1 items-center gap-1">{children}</div>;

/** Model catalog selection and reasoning controls, independent of tool state. */
export type AgentComposerModelProps = {
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

	if (props.isModelCatalogLoading) {
		return <Skeleton className="h-6 w-24 rounded" />;
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
