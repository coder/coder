import { Alert, AlertDescription, AlertTitle } from "#/components/Alert/Alert";
import { Link } from "#/components/Link/Link";
import { docs } from "#/utils/docs";

type BedrockInvokeModelAlertProps = {
	providerLabel: string;
};

/**
 * Warns that a Bedrock InvokeModel provider sends its configured model
 * upstream instead of the chat model's identifier.
 */
export const BedrockInvokeModelAlert: React.FC<
	BedrockInvokeModelAlertProps
> = ({ providerLabel }) => {
	return (
		<Alert severity="warning" prominent>
			<AlertTitle>The model identifier is not sent to AWS Bedrock</AlertTitle>
			<AlertDescription>
				{providerLabel} uses the Bedrock InvokeModel protocol, which replaces
				the requested model with the model configured on the provider. To choose
				the model per request, switch the provider to the Mantle protocol.{" "}
				<Link
					href={docs("/ai-coder/ai-gateway/providers#invokemodel")}
					target="_blank"
					rel="noreferrer"
				>
					Learn more about InvokeModel model selection
				</Link>
			</AlertDescription>
		</Alert>
	);
};
