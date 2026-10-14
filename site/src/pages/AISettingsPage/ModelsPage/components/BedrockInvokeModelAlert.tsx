import type { ChatModelProviderBedrockInvokeModel } from "#/api/typesGenerated";
import { Alert, AlertDescription, AlertTitle } from "#/components/Alert/Alert";
import { Link } from "#/components/Link/Link";
import { docs } from "#/utils/docs";

type BedrockInvokeModelAlertProps = {
	providerLabel: string;
	invokeModel: ChatModelProviderBedrockInvokeModel;
	modelIdentifier: string;
	className?: string;
};

// AI Gateway's InvokeModel path selects the small fast model when the
// requested model ID contains "haiku" and the primary model otherwise.
const isSmallFastModelRequest = (model: string): boolean =>
	model.includes("haiku");

export const BedrockInvokeModelAlert: React.FC<
	BedrockInvokeModelAlertProps
> = ({ providerLabel, invokeModel, modelIdentifier, className }) => {
	const smallFast = isSmallFastModelRequest(modelIdentifier.trim());
	const upstreamModel = smallFast
		? invokeModel.small_fast_model
		: invokeModel.model;
	const upstreamKind = smallFast ? "small fast model" : "model";

	return (
		<Alert severity="warning" prominent className={className}>
			<AlertTitle>The model identifier is not sent to AWS Bedrock</AlertTitle>
			<AlertDescription>
				{providerLabel} uses the Bedrock InvokeModel protocol, which replaces
				the requested model with the model configured on the provider.{" "}
				{upstreamModel ? (
					<>
						Requests for this model use the provider's {upstreamKind},{" "}
						<code className="font-mono">{upstreamModel}</code>.
					</>
				) : (
					<>Requests for this model use the provider's {upstreamKind}.</>
				)}{" "}
				To choose the model per request, switch the provider to the Mantle
				protocol.{" "}
				<Link href={docs("/ai-coder/ai-gateway/providers#invokemodel")}>
					Learn more about InvokeModel model selection
				</Link>
			</AlertDescription>
		</Alert>
	);
};
