import type { FC } from "react";
import type * as TypesGen from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { NotFound } from "#/components/NotFound/NotFound";
import type { ProviderState } from "#/modules/aiModels/providerStates";
import { pageTitle } from "#/utils/page";
import { ModelForm } from "../components/ModelForm";
import { ModelFormBackLink } from "../components/ModelFormHeader";

type UpdateModelPageViewProps =
	| { state: "error"; error: unknown }
	| { state: "notFound" }
	| {
			state: "loaded";
			model: TypesGen.ChatModel;
			refetchError?: unknown;
			currentDefaultModel?: TypesGen.ChatModel;
			providerStates: readonly ProviderState[];
			selectedProviderState: ProviderState | null;
			onProviderChange: (providerKey: string) => void;
			isSaving: boolean;
			isDeleting: boolean;
			canCreateModel: boolean;
			canUpdateModel: boolean;
			canDeleteModel: boolean;
			canShareModel: boolean;
			onUpdateModel: (
				modelId: string,
				req: TypesGen.UpdateChatModelRequest,
			) => Promise<unknown>;
			onDeleteModel: (modelId: string) => Promise<void>;
			onDuplicate: () => void;
			onToggleEnabled: (enabled: boolean) => void;
	  };

const UpdateModelPageView: FC<UpdateModelPageViewProps> = (props) => {
	if (props.state === "error") {
		return (
			<div className="flex flex-col items-start gap-4">
				<ModelFormBackLink />
				<ErrorAlert error={props.error} />
			</div>
		);
	}

	if (props.state === "notFound") {
		return <NotFound />;
	}

	const {
		model,
		refetchError,
		currentDefaultModel,
		providerStates,
		selectedProviderState,
		onProviderChange,
		isSaving,
		isDeleting,
		canCreateModel,
		canUpdateModel,
		canDeleteModel,
		canShareModel,
		onUpdateModel,
		onDeleteModel,
		onDuplicate,
		onToggleEnabled,
	} = props;
	return (
		<>
			<title>
				{pageTitle(model.display_name || model.model, "AI Settings")}
			</title>
			{refetchError != null && <ErrorAlert error={refetchError} />}
			<ModelForm
				key={model.id}
				editingModel={model}
				currentDefaultModel={currentDefaultModel}
				providerStates={providerStates}
				selectedProviderState={selectedProviderState}
				onProviderChange={onProviderChange}
				isSaving={isSaving}
				isDeleting={isDeleting}
				canUpdateModel={canUpdateModel}
				canShareModel={canShareModel}
				onCreateModel={async () => {}}
				onUpdateModel={onUpdateModel}
				onDeleteModel={canDeleteModel ? onDeleteModel : undefined}
				onDuplicate={canCreateModel ? onDuplicate : undefined}
				onToggleEnabled={canUpdateModel ? onToggleEnabled : undefined}
			/>
		</>
	);
};

export default UpdateModelPageView;
