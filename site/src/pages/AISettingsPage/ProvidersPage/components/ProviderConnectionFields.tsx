import type { AIProviderType } from "#/api/typesGenerated";
import { FormField } from "#/components/FormField/FormField";
import { Link } from "#/components/Link/Link";
import type { FormHelpers } from "#/utils/formUtils";
import { CredentialField } from "./CredentialField";

type FieldHelpersGetter = (
	fieldName: string,
	options?: { backendFieldName?: string },
) => FormHelpers;

const apiKeyCreationUrls: Partial<Record<AIProviderType, string>> = {
	anthropic: "https://platform.claude.com/settings/keys",
	google: "https://aistudio.google.com/apikey",
	openai: "https://platform.openai.com/api-keys",
	openrouter: "https://openrouter.ai/settings/keys",
	vercel:
		"https://vercel.com/d?to=%2F%5Bteam%5D%2F%7E%2Fai-gateway%2Fapi-keys&title=AI+Gateway+API+Keys",
};

type ProviderConnectionFieldsProps = {
	providerType: AIProviderType | "";
	getFieldHelpers: FieldHelpersGetter;
	baseUrlPlaceholder?: string;
	apiKeyPlaceholder?: string;
	onCredentialBlur: (field: "apiKey") => void;
	onCredentialFocus: (field: "apiKey") => void;
};

export const ProviderConnectionFields: React.FC<
	ProviderConnectionFieldsProps
> = ({
	providerType,
	getFieldHelpers,
	baseUrlPlaceholder,
	apiKeyPlaceholder,
	onCredentialBlur,
	onCredentialFocus,
}) => (
	<>
		<FormField
			required
			field={getFieldHelpers("baseUrl", {
				backendFieldName: "base_url",
			})}
			label="Endpoint"
			descriptionPosition="below"
			description={
				providerType === "copilot" ? (
					<>
						The base URL for your Copilot tier:{" "}
						<code>https://api.individual.githubcopilot.com</code>,{" "}
						<code>https://api.business.githubcopilot.com</code>, or{" "}
						<code>https://api.enterprise.githubcopilot.com</code>. Copilot
						authenticates with each user's GitHub OAuth token at request time,
						so there is no API key to configure here. This requires a GitHub
						external authentication provider to be configured.
					</>
				) : (
					"The base URL where the provider's API is hosted."
				)
			}
			className="w-full"
			placeholder={baseUrlPlaceholder}
		/>
		{providerType !== "copilot" && (
			<CredentialField
				required={providerType !== "anthropic"}
				label="API key"
				helpers={getFieldHelpers("apiKey")}
				onBlur={() => onCredentialBlur("apiKey")}
				onFocus={() => onCredentialFocus("apiKey")}
				placeholder={apiKeyPlaceholder}
				description={
					providerType && apiKeyCreationUrls[providerType] ? (
						<Link
							size="sm"
							href={apiKeyCreationUrls[providerType]}
							target="_blank"
							rel="noreferrer"
						>
							Create API key
						</Link>
					) : undefined
				}
			/>
		)}
	</>
);
