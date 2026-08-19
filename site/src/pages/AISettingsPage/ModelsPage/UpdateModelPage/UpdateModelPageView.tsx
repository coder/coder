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
	modelPricing?: TypesGen.AIModelPrice;
	pricingProvider?: string;
	isPricingLoading: boolean;
	isPricingFetching: boolean;
	pricingError: unknown;
	isPricingSaving: boolean;
	pricingSaveError: unknown;
	isPricingFeatureAvailable: boolean;
	canViewPricing: boolean;
	canEditPricing: boolean;
	onSavePricing: (price: TypesGen.AIModelPriceUpsert) => Promise<void>;
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
	modelPricing,
	pricingProvider,
	isPricingLoading,
	isPricingFetching,
	pricingError,
	isPricingSaving,
	pricingSaveError,
	isPricingFeatureAvailable,
	canViewPricing,
	canEditPricing,
	onSavePricing,
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
				modelPricing={modelPricing}
				pricingProvider={pricingProvider}
				isPricingLoading={isPricingLoading}
				isPricingFetching={isPricingFetching}
				pricingError={pricingError}
				isPricingSaving={isPricingSaving}
				pricingSaveError={pricingSaveError}
				isPricingFeatureAvailable={isPricingFeatureAvailable}
				canViewPricing={canViewPricing}
				canEditPricing={canEditPricing}
				onSavePricing={onSavePricing}
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
