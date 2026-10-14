import { Link } from "#/components/Link/Link";
import type { ModelSelectorOption } from "#/modules/aiModels/ModelSelector";
import { formatProviderLabel } from "#/utils/aiProviders";
import { docs } from "#/utils/docs";
import { ComposerNoticeRow } from "./ComposerNoticeRow";

type BedrockInvokeModelNoticeProps = {
	id: string;
	modelOption: ModelSelectorOption;
};

/**
 * Tells the user that the selected model's Bedrock InvokeModel provider runs
 * the model configured on the provider instead of the selected one.
 */
export const BedrockInvokeModelNotice: React.FC<
	BedrockInvokeModelNoticeProps
> = ({ id, modelOption }) => {
	const providerLabel =
		modelOption.providerLabel?.trim() ||
		formatProviderLabel(modelOption.provider);

	return (
		<ComposerNoticeRow id={id} tone="secondary">
			<span>
				{providerLabel} uses the Bedrock InvokeModel protocol, so requests use
				the model configured on the provider instead of{" "}
				{modelOption.displayName}.{" "}
				<Link
					href={docs("/ai-coder/ai-gateway/providers#invokemodel")}
					target="_blank"
					rel="noreferrer"
					size="sm"
				>
					Learn more about InvokeModel
				</Link>
			</span>
		</ComposerNoticeRow>
	);
};
