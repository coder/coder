import type { FC } from "react";
import { useLocation, useNavigate, useSearchParams } from "react-router";
import type * as TypesGen from "#/api/typesGenerated";
import { Alert, AlertDescription, AlertTitle } from "#/components/Alert/Alert";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Label } from "#/components/Label/Label";
import { Loader } from "#/components/Loader/Loader";
import {
	getOrganizationLabel,
	OrganizationAutocomplete,
} from "#/components/OrganizationAutocomplete/OrganizationAutocomplete";
import type { ProviderState } from "#/modules/aiModels/providerStates";
import { ModelForm } from "../components/ModelForm";
import { ModelFormBackLink } from "../components/ModelFormHeader";
import {
	creatableModelOrganizations,
	selectModelOrganizationPath,
	useOrganizationModels,
} from "../organizationModels";

interface AddModelPageViewProps {
	isLoading: boolean;
	loadError: unknown;
	refetchError: unknown;
	providerStates: readonly ProviderState[];
	selectedProviderState: ProviderState | null;
	duplicateSourceModel?: TypesGen.ChatModel;
	currentDefaultModel?: TypesGen.ChatModel;
	isSaving: boolean;
	onProviderChange: (providerKey: string) => void;
	onCreateModel: (req: TypesGen.CreateChatModelRequest) => Promise<unknown>;
}

const AddModelPageView: FC<AddModelPageViewProps> = ({
	isLoading,
	loadError,
	refetchError,
	providerStates,
	selectedProviderState,
	duplicateSourceModel,
	currentDefaultModel,
	isSaving,
	onProviderChange,
	onCreateModel,
}) => {
	const { organization, accessibleOrganizations, permissionsByOrganization } =
		useOrganizationModels();
	const location = useLocation();
	const navigate = useNavigate();
	const [searchParams] = useSearchParams();
	const creatableOrganizations = creatableModelOrganizations(
		accessibleOrganizations,
		permissionsByOrganization,
	);
	const organizationPicker = creatableOrganizations.length > 1 && (
		<div className="grid gap-1.5">
			<Label
				htmlFor="add-model-organization"
				className="leading-6 text-content-primary"
			>
				Organization
			</Label>
			<OrganizationAutocomplete
				id="add-model-organization"
				value={organization}
				ariaLabel={`Organization ${getOrganizationLabel(
					organization,
					accessibleOrganizations,
				)}`}
				options={creatableOrganizations}
				triggerClassName="w-60"
				optionsTabbable
				onChange={(nextOrganization) => {
					if (nextOrganization) {
						void navigate(
							selectModelOrganizationPath(
								location.pathname,
								nextOrganization,
								searchParams,
							),
						);
					}
				}}
			/>
		</div>
	);

	if (isLoading) {
		return <Loader fullscreen />;
	}

	if (loadError) {
		return (
			<div className="flex flex-col items-start gap-4">
				<ModelFormBackLink />
				<ErrorAlert error={loadError} />
				{organizationPicker}
			</div>
		);
	}

	if (!selectedProviderState) {
		return (
			<div className="flex flex-col items-start gap-4">
				<ModelFormBackLink />
				<Alert severity="warning">
					<AlertTitle>Provider not found</AlertTitle>
					<AlertDescription>
						The provider you are trying to add a model for is not available.
						Please try again.
					</AlertDescription>
				</Alert>
				{organizationPicker}
			</div>
		);
	}

	return (
		<div className="flex flex-col gap-4">
			{refetchError != null && <ErrorAlert error={refetchError} />}
			<ModelForm
				duplicateSourceModel={duplicateSourceModel}
				currentDefaultModel={currentDefaultModel}
				providerStates={providerStates}
				selectedProviderState={selectedProviderState}
				onProviderChange={onProviderChange}
				isSaving={isSaving}
				isDeleting={false}
				onCreateModel={onCreateModel}
				onUpdateModel={async () => {}}
			/>
		</div>
	);
};

export default AddModelPageView;
