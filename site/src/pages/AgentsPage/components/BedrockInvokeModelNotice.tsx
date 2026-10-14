import { TriangleAlertIcon } from "lucide-react";
import { Link } from "#/components/Link/Link";
import type { ModelSelectorOption } from "#/modules/aiModels/ModelSelector";
import { docs } from "#/utils/docs";

type BedrockInvokeModelNoticeProps = {
	model: ModelSelectorOption;
};

/**
 * Tells the user that the selected model's Bedrock InvokeModel provider runs
 * the model configured on the provider instead of the selected one.
 */
export const BedrockInvokeModelNotice: React.FC<
	BedrockInvokeModelNoticeProps
> = ({ model }) => {
	return (
		<div className="flex items-start gap-1.5 border-b border-border/70 px-3 py-1.5 text-xs text-content-secondary">
			<TriangleAlertIcon className="mt-px size-3.5 shrink-0 text-content-warning" />
			<span>
				{model.providerLabel || "This provider"} uses the Bedrock InvokeModel
				protocol, so requests use the model configured on the provider instead
				of {model.displayName}.{" "}
				<Link
					href={docs("/ai-coder/ai-gateway/providers#invokemodel")}
					size="sm"
				>
					Learn more
				</Link>
			</span>
		</div>
	);
};
