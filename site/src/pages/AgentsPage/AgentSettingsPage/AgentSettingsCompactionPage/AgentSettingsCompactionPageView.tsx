import type * as TypesGen from "#/api/typesGenerated";
import { SectionHeader } from "../../components/SectionHeader";
import { UserCompactionThresholdSettings } from "../../components/UserCompactionThresholdSettings";
import type {
	CompactionTriggerLoadError,
	OrganizationCompactionTrigger,
} from "../../utils/compactionTriggers";
import type { ProviderInfo } from "../../utils/modelOptions";

export type AgentSettingsCompactionPageViewProps = {
	models: readonly TypesGen.ChatModel[];
	providerInfoByID: ReadonlyMap<string, ProviderInfo>;
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
	providerInfoByID,
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
				providerInfoByID={providerInfoByID}
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
