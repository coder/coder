import { ArrowLeftIcon } from "lucide-react";
import type { FC } from "react";
import { Link } from "react-router";
import type * as TypesGen from "#/api/typesGenerated";
import { Alert, AlertDescription, AlertTitle } from "#/components/Alert/Alert";
import { Button } from "#/components/Button/Button";
import { Loader } from "#/components/Loader/Loader";
import type { ProviderState } from "#/modules/aiModels/providerStates";
import { ModelForm } from "../components/ModelForm";

interface AddModelPageViewProps {
	isLoading: boolean;
	providerStates: readonly ProviderState[];
	selectedProviderState: ProviderState | null;
	duplicateSourceModel?: TypesGen.ChatModelConfig;
	currentDefaultModel?: TypesGen.ChatModelConfig;
	isSaving: boolean;
	onProviderChange: (providerKey: string) => void;
	onCreateModel: (
		req: TypesGen.CreateChatModelConfigRequest,
	) => Promise<unknown>;
}

const AddModelPageView: FC<AddModelPageViewProps> = ({
	isLoading,
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

	if (!selectedProviderState) {
		return (
			<div className="flex flex-col items-start gap-4">
				<Link to="/ai/settings/models" className="-ml-3">
					<Button variant="subtle">
						<ArrowLeftIcon />
						<span>Back to models</span>
					</Button>
				</Link>
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
	);
};

export default AddModelPageView;
