import type { FC } from "react";
import type * as TypesGen from "#/api/typesGenerated";
import { SectionHeader } from "./components/SectionHeader";
import { UserCompactionThresholdSettings } from "./components/UserCompactionThresholdSettings";

export type AgentSettingsCompactionPageViewProps = {
	models: readonly TypesGen.ChatModel[] | undefined;
	providerTypeByID: ReadonlyMap<string, string>;
	organizations: readonly TypesGen.Organization[];
	compactionModelIDByOrganization?: ReadonlyMap<string, string>;
	modelsError: unknown;
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

export const AgentSettingsCompactionPageView: FC<
	AgentSettingsCompactionPageViewProps
> = ({
	models,
	providerTypeByID,
	organizations,
	compactionModelIDByOrganization,
	modelsError,
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
				models={models ?? []}
				providerTypeByID={providerTypeByID}
				organizations={organizations}
				compactionModelIDByOrganization={compactionModelIDByOrganization}
				modelsError={modelsError}
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
