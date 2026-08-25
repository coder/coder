import type { FC } from "react";
import type * as TypesGen from "#/api/typesGenerated";
import { Alert, AlertDescription, AlertTitle } from "#/components/Alert/Alert";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Loader } from "#/components/Loader/Loader";
import type { ProviderState } from "#/modules/aiModels/providerStates";
import { ModelForm } from "../components/ModelForm";
import { ModelFormBackLink } from "../components/ModelFormHeader";

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
	if (isLoading) {
		return <Loader fullscreen />;
	}

	if (loadError) {
		return (
			<div className="flex flex-col items-start gap-4">
				<ModelFormBackLink />
				<ErrorAlert error={loadError} />
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
