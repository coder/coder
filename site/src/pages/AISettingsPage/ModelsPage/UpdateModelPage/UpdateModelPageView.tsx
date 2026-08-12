import type { FC } from "react";
import type * as TypesGen from "#/api/typesGenerated";
import type { ProviderState } from "#/modules/aiModels/providerStates";
import { pageTitle } from "#/utils/page";
import { ModelForm } from "../components/ModelForm";

interface UpdateModelPageViewProps {
	model: TypesGen.ChatModelConfig;
	currentDefaultModel?: TypesGen.ChatModelConfig;
	providerStates: readonly ProviderState[];
	selectedProviderState: ProviderState | null;
	onProviderChange: (providerKey: string) => void;
	isSaving: boolean;
	isDeleting: boolean;
	onUpdateModel: (
		modelConfigId: string,
		req: TypesGen.UpdateChatModelConfigRequest,
	) => Promise<unknown>;
	onDeleteModel: (modelConfigId: string) => Promise<void>;
	onDuplicate: () => void;
	onToggleEnabled: (enabled: boolean) => void;
}

const UpdateModelPageView: FC<UpdateModelPageViewProps> = ({
	model,
	currentDefaultModel,
	providerStates,
	selectedProviderState,
	onProviderChange,
	isSaving,
	isDeleting,
	onUpdateModel,
	onDeleteModel,
	onDuplicate,
	onToggleEnabled,
}) => {
	return (
		<>
			<title>
				{pageTitle(model.display_name || model.model, "AI Settings")}
			</title>
			<ModelForm
				key={model.id}
				editingModel={model}
				currentDefaultModel={currentDefaultModel}
				providerStates={providerStates}
				selectedProviderState={selectedProviderState}
				onProviderChange={onProviderChange}
				isSaving={isSaving}
				isDeleting={isDeleting}
				onCreateModel={async () => {}}
				onUpdateModel={onUpdateModel}
				onDeleteModel={onDeleteModel}
				onDuplicate={onDuplicate}
				onToggleEnabled={onToggleEnabled}
			/>
		</>
	);
};

export default UpdateModelPageView;
