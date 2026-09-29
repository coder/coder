import type { AIProviderType } from "#/api/typesGenerated";
import { FormField } from "#/components/FormField/FormField";
import type { FormHelpers } from "#/utils/formUtils";
import { CredentialField } from "./CredentialField";

type FieldHelpersGetter = (
	fieldName: string,
	options?: { backendFieldName?: string },
) => FormHelpers;

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
			description={
				providerType === "copilot" ? (
					<>
						The base URL for your Copilot tier:{" "}
						<code>https://api.individual.githubcopilot.com</code>,{" "}
						<code>https://api.business.githubcopilot.com</code>, or{" "}
						<code>https://api.enterprise.githubcopilot.com</code>.
					</>
				) : (
					"The base URL where the provider's API is hosted."
				)
			}
			className="w-full"
			placeholder={baseUrlPlaceholder}
		/>
		{providerType === "copilot" ? (
			<p className="text-sm text-content-secondary m-0">
				Copilot authenticates with each user's GitHub OAuth token at request
				time, so there is no API key to configure here. This requires a GitHub
				external authentication provider to be configured.
			</p>
		) : (
			<CredentialField
				required={providerType !== "anthropic"}
				label="API key"
				helpers={getFieldHelpers("apiKey")}
				onBlur={() => onCredentialBlur("apiKey")}
				onFocus={() => onCredentialFocus("apiKey")}
				autoComplete="new-password"
				placeholder={apiKeyPlaceholder}
			/>
		)}
	</>
);
