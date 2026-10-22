import { Link } from "#/components/Link/Link";
import {
	getProviderLabel,
	type ModelSelectorOption,
} from "#/modules/aiModels/ModelSelector";
import { bedrockInvokeModelDocsPath } from "#/utils/aiProviders";
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
	return (
		<ComposerNoticeRow id={id} tone="secondary">
			<span>
				The {getProviderLabel(modelOption)} provider uses the Bedrock
				InvokeModel protocol, so requests use the model configured on the
				provider instead of {modelOption.displayName}.{" "}
				<Link
					href={docs(bedrockInvokeModelDocsPath)}
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
