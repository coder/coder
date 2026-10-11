import type * as TypesGen from "#/api/typesGenerated";
import type {
	CompactionTriggerLoadError,
	OrganizationCompactionTrigger,
} from "./compactionTriggers";
import { SectionHeader } from "./components/SectionHeader";
import { UserCompactionThresholdSettings } from "./components/UserCompactionThresholdSettings";

export type AgentSettingsCompactionPageViewProps = {
	models: readonly TypesGen.ChatModel[];
	providerTypeByID: ReadonlyMap<string, string>;
	organizations: readonly TypesGen.Organization[];
	compactionTriggersByOrganizationID: ReadonlyMap<
		string,
		OrganizationCompactionTrigger
	>;
	modelsError: unknown;
	compactionTriggerLoadErrors: readonly CompactionTriggerLoadError[];
	isLoadingModels: boolean;
	thresholds: readonly TypesGen.UserChatCompactionThreshold[] | undefined;
	isThresholdsLoading: boolean;
	thresholdsError: unknown;
	onSaveThreshold: (
		modelId: string,
		thresholdPercent: number,
	) => Promise<unknown>;
	onResetThreshold: (modelId: string) => Promise<unknown>;
};

export const AgentSettingsCompactionPageView: React.FC<
	AgentSettingsCompactionPageViewProps
> = ({
	models,
	providerTypeByID,
	organizations,
	compactionTriggersByOrganizationID,
	modelsError,
	compactionTriggerLoadErrors,
	isLoadingModels,
	thresholds,
	isThresholdsLoading,
	thresholdsError,
	onSaveThreshold,
	onResetThreshold,
}) => {
	return (
		<div className="flex flex-col gap-8">
			<SectionHeader
				label="Compaction"
				description="Customize when conversations with models are automatically compacted."
			/>
			<UserCompactionThresholdSettings
				models={models}
				providerTypeByID={providerTypeByID}
				organizations={organizations}
				compactionTriggersByOrganizationID={compactionTriggersByOrganizationID}
				modelsError={modelsError}
				compactionTriggerLoadErrors={compactionTriggerLoadErrors}
				isLoadingModels={isLoadingModels}
				thresholds={thresholds}
				isThresholdsLoading={isThresholdsLoading}
				thresholdsError={thresholdsError}
				onSaveThreshold={onSaveThreshold}
				onResetThreshold={onResetThreshold}
			/>
		</div>
	);
};
