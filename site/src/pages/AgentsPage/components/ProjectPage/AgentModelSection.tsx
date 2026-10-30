import { useMutation, useQuery, useQueryClient } from "react-query";
import { updateChatProject } from "#/api/queries/chatProjects";
import { chatModels } from "#/api/queries/chats";
import type { ChatProject, ChatProjectModelStatus } from "#/api/typesGenerated";
import { Alert, AlertDescription } from "#/components/Alert/Alert";
import { Spinner } from "#/components/Spinner/Spinner";
import {
	ModelSelector,
	type ModelSelectorOption,
} from "#/modules/aiModels/ModelSelector";
import {
	getUsableDefaultModelIDForOrganization,
	resolveModelSelector,
} from "../../utils/modelOptions";
import {
	SectionError,
	SectionSaveError,
	SettingsCard,
	SettingsCardRow,
	SettingsCardRowText,
	SettingsSection,
} from "./ProjectSection";
import { useSingleFlight } from "./useSingleFlight";

type AgentModelSectionProps = {
	readonly project: ChatProject;
};

/** Picks the model that new chats in an agent use. A pick saves at once. */
export const AgentModelSection: React.FC<AgentModelSectionProps> = ({
	project,
}) => {
	const queryClient = useQueryClient();
	const modelsQuery = useQuery(chatModels(project.organization_id));
	const updateMutation = useMutation(updateChatProject(queryClient));
	const singleFlight = useSingleFlight();

	const { options, isModelCatalogLoading } = resolveModelSelector(
		project.organization_id,
		modelsQuery,
	);
	const defaultModelID = getUsableDefaultModelIDForOrganization(
		modelsQuery.data?.models ?? [],
		options,
		project.organization_id,
	);

	return (
		<AgentModelSectionView
			options={options}
			defaultModelName={
				options.find((option) => option.id === defaultModelID)?.displayName
			}
			modelConfigId={project.model_config_id ?? ""}
			modelStatus={project.model_status}
			modelStatusReason={project.model_status_reason}
			modelDisplayName={project.model_display_name}
			isLoading={isModelCatalogLoading}
			modelsError={modelsQuery.error}
			onRetryModels={() => void modelsQuery.refetch()}
			canEdit={project.permissions.edit}
			isSaving={updateMutation.isPending}
			saveError={updateMutation.error}
			onChange={(modelConfigId) =>
				singleFlight.run((onSettled) =>
					updateMutation.mutate(
						{ project, request: { model_config_id: modelConfigId } },
						{ onSettled },
					),
				)
			}
		/>
	);
};

type AgentModelSectionViewProps = {
	/** The models the picker offers. */
	readonly options: readonly ModelSelectorOption[];
	/** The name of the model the deployment default currently is. */
	readonly defaultModelName: string | undefined;
	/** The model the agent is set to use, or empty for the default. */
	readonly modelConfigId: string;
	readonly modelStatus: ChatProjectModelStatus;
	/** A full sentence saying why the model is unavailable. */
	readonly modelStatusReason: ChatProject["model_status_reason"];
	/** The name of the set model, including a deleted one. */
	readonly modelDisplayName: ChatProject["model_display_name"];
	readonly isLoading: boolean;
	readonly modelsError: unknown;
	readonly onRetryModels: () => void;
	readonly canEdit: boolean;
	readonly isSaving: boolean;
	readonly saveError: unknown;
	/** Called with a model ID, or "" to use the default model. */
	readonly onChange: (modelConfigId: string) => void;
};

/** The model picker of an agent, with a warning when its model is gone. */
export const AgentModelSectionView: React.FC<AgentModelSectionViewProps> = ({
	options,
	defaultModelName,
	modelConfigId,
	modelStatus,
	modelStatusReason,
	modelDisplayName,
	isLoading,
	modelsError,
	onRetryModels,
	canEdit,
	isSaving,
	saveError,
	onChange,
}) => {
	const defaultLabel = defaultModelName
		? `Default model (${defaultModelName})`
		: "Default model";
	// The set model stays visible even when the picker no longer lists it.
	const setLabel = modelDisplayName || "Unknown model";
	let selectedLabel = defaultLabel;
	if (isLoading) {
		selectedLabel = "Loading models...";
	} else if (modelConfigId) {
		selectedLabel = setLabel;
	}
	// Without a model list the picker has nothing to offer but the default.
	const hasModelsLoadFailed = Boolean(modelsError) && options.length === 0;

	return (
		<SettingsSection
			title="Model"
			description="The model new chats in this agent start with."
		>
			{Boolean(modelsError) && (
				<SectionError error={modelsError} onRetry={onRetryModels} />
			)}
			{Boolean(saveError) && <SectionSaveError error={saveError} />}
			{modelStatus === "unavailable" && (
				<Alert severity="warning">
					<AlertDescription>
						{setLabel} is no longer available.{" "}
						{modelStatusReason ? `${modelStatusReason} ` : ""}New chats start
						with the default model until you pick another one.
					</AlertDescription>
				</Alert>
			)}
			<SettingsCard as="div">
				<SettingsCardRow as="div">
					<SettingsCardRowText
						title="Starting model"
						description="Anyone starting a chat can pick another."
					/>
					{canEdit ? (
						<div className="flex w-72 max-w-full shrink-0 items-center gap-2">
							<Spinner loading={isSaving} />
							<ModelSelector
								options={options}
								value={modelConfigId}
								onValueChange={onChange}
								triggerAriaLabel="Agent model"
								disabled={isLoading || hasModelsLoadFailed}
								unsetLabel={defaultLabel}
								placeholder={selectedLabel}
								emptyMessage="No enabled models found."
								variant="field"
							/>
						</div>
					) : (
						<span className="shrink-0 text-sm text-content-secondary">
							{selectedLabel}
						</span>
					)}
				</SettingsCardRow>
			</SettingsCard>
		</SettingsSection>
	);
};
