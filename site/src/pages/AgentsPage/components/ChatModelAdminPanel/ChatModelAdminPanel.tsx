import type { FC } from "react";

import type * as TypesGen from "#/api/typesGenerated";
import { Alert, AlertDescription, AlertTitle } from "#/components/Alert/Alert";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Spinner } from "#/components/Spinner/Spinner";
import { deriveProviderStates } from "#/modules/aiModels/providerStates";
import { cn } from "#/utils/cn";
import { ModelsSection } from "./ModelsSection";
import { ProvidersSection } from "./ProvidersSection";

export type CreateProviderResult = { id: string };

export type ChatModelAdminSection = "providers" | "models";

interface ChatModelAdminPanelProps {
	className?: string;
	section?: ChatModelAdminSection;
	sectionLabel?: string;
	sectionDescription?: string;
	// Data from queries.
	providerConfigsData: TypesGen.ChatProviderConfig[] | undefined;
	modelConfigsData: TypesGen.ChatModelConfig[] | undefined;
	modelCatalogData: TypesGen.ChatModelsResponse | undefined;
	isLoading: boolean;
	// Query error states.
	providerConfigsError: Error | null;
	modelConfigsError: Error | null;
	modelCatalogError: Error | null;
	// Provider mutation handlers.
	onCreateProvider: (
		req: TypesGen.CreateChatProviderConfigRequest,
	) => Promise<CreateProviderResult>;
	onUpdateProvider: (
		providerConfigId: string,
		req: TypesGen.UpdateChatProviderConfigRequest,
	) => Promise<unknown>;
	onDeleteProvider: (providerConfigId: string) => Promise<void>;
	isProviderMutationPending: boolean;
	providerMutationError: Error | null;
	// Model mutation handlers.
	onCreateModel: (
		req: TypesGen.CreateChatModelConfigRequest,
	) => Promise<unknown>;
	onUpdateModel: (
		modelConfigId: string,
		req: TypesGen.UpdateChatModelConfigRequest,
	) => Promise<unknown>;
	onDeleteModel: (modelConfigId: string) => Promise<void>;
	isCreatingModel: boolean;
	isUpdatingModel: boolean;
	isDeletingModel: boolean;
	modelMutationError: Error | null;
}

export const ChatModelAdminPanel: FC<ChatModelAdminPanelProps> = ({
	className,
	section = "providers",
	sectionLabel,
	sectionDescription,
	providerConfigsData,
	modelConfigsData,
	modelCatalogData,
	isLoading,
	providerConfigsError,
	modelConfigsError,
	modelCatalogError,
	onCreateProvider,
	onUpdateProvider,
	onDeleteProvider,
	isProviderMutationPending,
	providerMutationError,
	onCreateModel,
	onUpdateModel,
	onDeleteModel,
	isCreatingModel,
	isUpdatingModel,
	isDeletingModel,
	modelMutationError,
}) => {
	const modelConfigs = (modelConfigsData ?? []).slice().sort((a, b) => {
		const cmp = a.provider.localeCompare(b.provider);
		return cmp !== 0 ? cmp : a.model.localeCompare(b.model);
	});
	const providerStates = deriveProviderStates(
		modelConfigs,
		providerConfigsData,
		modelCatalogData,
	);

	const providerConfigsUnavailable = providerConfigsData === null;
	const modelConfigsUnavailable = modelConfigsData === null;

	return (
		<div className={cn("flex min-h-full flex-col", className)}>
			{isLoading && (
				<div className="flex items-center gap-1.5 text-xs text-content-secondary">
					<Spinner className="h-4 w-4" loading />
					Loading
				</div>
			)}

			<div className="flex flex-1 flex-col gap-8">
				{section === "providers" ? (
					<ProvidersSection
						sectionLabel={sectionLabel}
						sectionDescription={sectionDescription}
						providerStates={providerStates}
						providerConfigsUnavailable={providerConfigsUnavailable}
						isProviderMutationPending={isProviderMutationPending}
						onCreateProvider={onCreateProvider}
						onUpdateProvider={onUpdateProvider}
						onDeleteProvider={onDeleteProvider}
					/>
				) : (
					<ModelsSection
						sectionLabel={sectionLabel}
						sectionDescription={sectionDescription}
						providerStates={providerStates}
						modelConfigs={modelConfigs}
						modelConfigsUnavailable={modelConfigsUnavailable}
						isCreating={isCreatingModel}
						isUpdating={isUpdatingModel}
						isDeleting={isDeletingModel}
						onCreateModel={onCreateModel}
						onUpdateModel={onUpdateModel}
						onDeleteModel={onDeleteModel}
					/>
				)}
			</div>
			{providerConfigsError && <ErrorAlert error={providerConfigsError} />}
			{modelConfigsError && <ErrorAlert error={modelConfigsError} />}
			{modelCatalogError && <ErrorAlert error={modelCatalogError} />}
			{providerMutationError && <ErrorAlert error={providerMutationError} />}
			{modelMutationError && <ErrorAlert error={modelMutationError} />}

			{providerConfigsUnavailable && (
				<Alert severity="info">
					<AlertTitle>
						Chat provider admin API is unavailable on this deployment.
					</AlertTitle>
					<AlertDescription>
						/api/v2/chats/providers is missing.
					</AlertDescription>
				</Alert>
			)}

			{modelConfigsUnavailable && (
				<Alert severity="info">
					<AlertTitle>
						Chat model admin API is unavailable on this deployment.
					</AlertTitle>
					<AlertDescription>
						/api/v2/chats/model-configs is missing.
					</AlertDescription>
				</Alert>
			)}
		</div>
	);
};
