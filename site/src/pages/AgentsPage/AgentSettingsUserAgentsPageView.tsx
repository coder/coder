import type { FC } from "react";
import type * as TypesGen from "#/api/typesGenerated";
import { Alert, AlertDescription } from "#/components/Alert/Alert";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Button } from "#/components/Button/Button";
import {
	getOrganizationLabel,
	OrganizationAutocomplete,
} from "#/components/OrganizationAutocomplete/OrganizationAutocomplete";
import type { ModelSelectorOption } from "#/modules/aiModels/ModelSelector";
import { PersonalModelOverrideRow } from "./components/PersonalModelOverrideRow";
import { SectionHeader } from "./components/SectionHeader";

export interface AgentSettingsUserAgentsPageViewProps {
	overridesData?: TypesGen.UserChatPersonalModelOverridesResponse;
	overridesError: unknown;
	onRetryOverrides: () => void;
	isRetryingOverrides: boolean;
	isLoading: boolean;
	modelOptions: readonly ModelSelectorOption[];
	models: readonly TypesGen.ChatModel[];
	modelsError: unknown;
	organizations: readonly TypesGen.Organization[];
	selectedOrganization: TypesGen.Organization | undefined;
	onSelectOrganization: (organization: TypesGen.Organization) => void;
	onSaveOverride: (
		args: {
			organizationId: string;
			context: TypesGen.ChatPersonalModelOverrideContext;
			req: TypesGen.UpdateUserChatPersonalModelOverrideRequest;
		},
		options?: { onSuccess?: () => void; onError?: () => void },
	) => void;
	isSaving: boolean;
	saveContext?: TypesGen.ChatPersonalModelOverrideContext;
}

// Generated ChatPersonalModelOverrideContexts is alphabetical, not display order.
const PERSONAL_OVERRIDE_CONTEXTS = ["root", "general", "explore"] as const;

export const AgentSettingsUserAgentsPageView: FC<
	AgentSettingsUserAgentsPageViewProps
> = ({
	overridesData,
	overridesError,
	onRetryOverrides,
	isRetryingOverrides,
	isLoading,
	modelOptions,
	models,
	modelsError,
	organizations,
	selectedOrganization,
	onSelectOrganization,
	onSaveOverride,
	isSaving,
	saveContext,
}) => {
	const personalOverridesEnabled = overridesData?.enabled ?? true;
	const hasNoOrganizationModels =
		selectedOrganization !== undefined &&
		!isLoading &&
		!modelsError &&
		modelOptions.length === 0;

	const isDisabled =
		isLoading ||
		!personalOverridesEnabled ||
		!selectedOrganization ||
		hasNoOrganizationModels;

	return (
		<div className="flex flex-col gap-8">
			<SectionHeader
				label="Agents"
				description="Choose personal model defaults for root agents and delegated agents."
			/>
			{organizations.length > 1 && selectedOrganization && (
				<OrganizationAutocomplete
					value={selectedOrganization}
					options={organizations}
					ariaLabel={`Organization ${getOrganizationLabel(selectedOrganization, organizations)}`}
					triggerClassName="w-60"
					optionsTabbable
					onChange={(organization) => {
						if (organization) {
							onSelectOrganization(organization);
						}
					}}
				/>
			)}
			{Boolean(overridesError) && (
				<div className="flex flex-col gap-2">
					<ErrorAlert error={overridesError} />
					<Button
						disabled={isRetryingOverrides}
						onClick={onRetryOverrides}
						size="sm"
						type="button"
						variant="outline"
					>
						Retry
					</Button>
				</div>
			)}
			{!personalOverridesEnabled && (
				<Alert severity="info">
					<AlertDescription>
						Personal model overrides are disabled by an administrator. Saved
						values are shown for reference, but changes cannot be saved.
					</AlertDescription>
				</Alert>
			)}
			{!selectedOrganization && (
				<Alert severity="info">
					<AlertDescription>
						You do not have access to any organizations. Personal model
						overrides cannot be changed.
					</AlertDescription>
				</Alert>
			)}
			{hasNoOrganizationModels && (
				<Alert severity="info">
					<AlertDescription>
						The selected organization has no available chat models. Ask an
						organization administrator to add and enable a model before you
						choose a specific model.
					</AlertDescription>
				</Alert>
			)}
			{PERSONAL_OVERRIDE_CONTEXTS.map((context) => (
				<PersonalModelOverrideRow
					key={context}
					context={context}
					overrideData={overridesData?.[context]}
					deploymentDefault={
						context === "root"
							? undefined
							: overridesData?.deployment_defaults[context]
					}
					modelOptions={modelOptions}
					models={models}
					modelsError={modelsError}
					isLoading={isLoading}
					onSave={(req, options) => {
						if (!selectedOrganization) {
							return;
						}
						onSaveOverride(
							{
								organizationId: selectedOrganization.id,
								context,
								req,
							},
							options,
						);
					}}
					isSaving={isSaving && saveContext === context}
					disabled={isDisabled}
				/>
			))}
		</div>
	);
};
