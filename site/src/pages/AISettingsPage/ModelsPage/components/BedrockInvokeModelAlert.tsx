import type { ChatModelProviderBedrockInvokeModel } from "#/api/typesGenerated";
import { Alert, AlertDescription, AlertTitle } from "#/components/Alert/Alert";
import { Link } from "#/components/Link/Link";
import { docs } from "#/utils/docs";

type BedrockInvokeModelAlertProps = {
	providerLabel: string;
	configuredModels: ChatModelProviderBedrockInvokeModel;
	modelIdentifier: string;
};

/**
 * Warns that a Bedrock InvokeModel provider sends its configured model
 * upstream instead of the chat model's identifier.
 */
export const BedrockInvokeModelAlert: React.FC<
	BedrockInvokeModelAlertProps
> = ({ providerLabel, configuredModels, modelIdentifier }) => {
	// Mirrors AI Gateway's isSmallFastModel in aibridge/intercept/messages.
	const smallFast = modelIdentifier.includes("haiku");
	const upstreamModel = smallFast
		? configuredModels.small_fast_model
		: configuredModels.model;
	const upstreamKind = smallFast ? "small-fast model" : "model";

	return (
		<Alert severity="warning" prominent className="sm:col-span-2">
			<AlertTitle>The model identifier is not sent to AWS Bedrock</AlertTitle>
			<AlertDescription>
				{providerLabel} uses the Bedrock InvokeModel protocol, which replaces
				the requested model with the model configured on the provider. Requests
				for this model use the provider's {upstreamKind}
				{upstreamModel && (
					<>
						, <code className="font-mono">{upstreamModel}</code>
					</>
				)}
				. To choose the model per request, switch the provider to the Mantle
				protocol.{" "}
				<Link href={docs("/ai-coder/ai-gateway/providers#invokemodel")}>
					Learn more about InvokeModel model selection
				</Link>
			</AlertDescription>
		</Alert>
	);
};
