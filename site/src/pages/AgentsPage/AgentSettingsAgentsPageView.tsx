import type { FC } from "react";
import type * as TypesGen from "#/api/typesGenerated";
import { ExploreModelOverrideSettings } from "./components/ExploreModelOverrideSettings";
import { SectionHeader } from "./components/SectionHeader";

interface MutationCallbacks {
	onSuccess?: () => void;
	onError?: () => void;
}

export interface AgentSettingsAgentsPageViewProps {
	exploreModelOverrideData:
		| TypesGen.ChatExploreModelOverrideResponse
		| undefined;
	modelConfigsData: TypesGen.ChatModelConfig[] | undefined;
	modelConfigsError: unknown;
	isLoadingModelConfigs: boolean;
	onSaveExploreModelOverride: (
		req: TypesGen.UpdateChatExploreModelOverrideRequest,
		options?: MutationCallbacks,
	) => void;
	isSavingExploreModelOverride: boolean;
	isSaveExploreModelOverrideError: boolean;
}

export const AgentSettingsAgentsPageView: FC<
	AgentSettingsAgentsPageViewProps
> = ({
	exploreModelOverrideData,
	modelConfigsData,
	modelConfigsError,
	isLoadingModelConfigs,
	onSaveExploreModelOverride,
	isSavingExploreModelOverride,
	isSaveExploreModelOverrideError,
}) => {
	return (
		<div className="flex flex-col gap-8">
			<SectionHeader
				label="Agents"
				description="Configure defaults for delegated agents and other agent-specific capabilities."
			/>
			<div className="flex flex-col gap-3">
				<SectionHeader
					label="Explore subagent model"
					description="Optional deployment-wide model override for read-only Explore subagents."
					level="section"
				/>
				<ExploreModelOverrideSettings
					exploreModelOverrideData={exploreModelOverrideData}
					modelConfigs={modelConfigsData ?? []}
					modelConfigsError={modelConfigsError}
					isLoadingModelConfigs={isLoadingModelConfigs}
					onSaveExploreModelOverride={onSaveExploreModelOverride}
					isSavingExploreModelOverride={isSavingExploreModelOverride}
					isSaveExploreModelOverrideError={isSaveExploreModelOverrideError}
					showHeader={false}
				/>
			</div>
		</div>
	);
};
