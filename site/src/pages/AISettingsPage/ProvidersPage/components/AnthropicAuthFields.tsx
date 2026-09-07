import { type FC, useId } from "react";
import type { AIProviderClaudePlatformAWSAuthMode } from "#/api/typesGenerated";
import { CodeExample } from "#/components/CodeExample/CodeExample";
import { FormField } from "#/components/FormField/FormField";
import { Label } from "#/components/Label/Label";
import { Link as DocsLink } from "#/components/Link/Link";
import { RadioGroup, RadioGroupItem } from "#/components/RadioGroup/RadioGroup";
import { docs } from "#/utils/docs";
import type { FormHelpers } from "#/utils/formUtils";
import { CredentialField } from "./CredentialField";
import {
	CLAUDE_PLATFORM_DEFAULT_REGION,
	claudePlatformBaseUrl,
} from "./claudePlatform";

/**
 * How an `anthropic` provider authenticates. Claude Platform for AWS is a
 * settings variant on the same provider type, so it is a choice inside the
 * Anthropic form rather than a separate entry in the provider type list.
 */
export type AnthropicAuthMethod = "api_key" | "claude_platform_aws";

/** The `getFormHelpers(form, error)` accessor bound by the parent form. */
type FieldHelpersGetter = (
	fieldName: string,
	options?: { backendFieldName?: string },
) => FormHelpers;

/** Credential inputs whose masked placeholder the parent form manages. */
type CredentialFieldName = "apiKey" | "accessKey" | "accessKeySecret";

const CLAUDE_PLATFORM_DOCS_HREF =
	"/ai-coder/ai-gateway/providers#claude-platform-for-aws";

type RadioOptionProps = {
	id: string;
	value: string;
	label: string;
	description: React.ReactNode;
};

const RadioOption: FC<RadioOptionProps> = ({
	id,
	value,
	label,
	description,
}) => {
	const descriptionId = `${id}-description`;
	return (
		<div className="flex items-start gap-3">
			<RadioGroupItem
				value={value}
				id={id}
				aria-describedby={descriptionId}
				className="mt-1"
			/>
			<div className="flex flex-col gap-1">
				<Label htmlFor={id}>{label}</Label>
				<span id={descriptionId} className="text-xs text-content-secondary">
					{description}
				</span>
			</div>
		</div>
	);
};

type AnthropicAuthMethodFieldProps = {
	value: AnthropicAuthMethod;
	/** The method is fixed once the provider exists; changing it is a new provider. */
	disabled?: boolean;
	onChange: (method: AnthropicAuthMethod) => void;
};

export const AnthropicAuthMethodField: FC<AnthropicAuthMethodFieldProps> = ({
	value,
	disabled = false,
	onChange,
}) => {
	const labelId = useId();
	return (
		<div className="flex flex-col gap-2">
			<Label id={labelId}>Authentication</Label>
			<RadioGroup
				className="gap-3"
				aria-labelledby={labelId}
				value={value}
				disabled={disabled}
				onValueChange={(next) => onChange(next as AnthropicAuthMethod)}
			>
				<RadioOption
					id={`${labelId}-api-key`}
					value="api_key"
					label="API key"
					description="Call the Anthropic API directly with an Anthropic API key."
				/>
				<RadioOption
					id={`${labelId}-claude-platform`}
					value="claude_platform_aws"
					label="Claude Platform for AWS"
					description={
						<>
							Call Anthropic's Messages API hosted on AWS, billed through an AWS
							account.{" "}
							<DocsLink
								size="sm"
								href={docs(CLAUDE_PLATFORM_DOCS_HREF)}
								target="_blank"
								rel="noreferrer"
							>
								View docs
							</DocsLink>
						</>
					}
				/>
			</RadioGroup>
			{disabled && (
				<p className="text-xs text-content-secondary m-0">
					The authentication method is fixed after the provider is created. Add
					a new provider to use the other method.
				</p>
			)}
		</div>
	);
};

type ClaudePlatformFieldsProps = {
	editing: boolean;
	authMode: AIProviderClaudePlatformAWSAuthMode;
	/** Server-generated STS external ID, shown read-only when a role is assumed. */
	awsExternalId?: string;
	getFieldHelpers: FieldHelpersGetter;
	onAuthModeChange: (mode: AIProviderClaudePlatformAWSAuthMode) => void;
	onRegionChange: (region: string) => void;
	onCredentialBlur: (field: CredentialFieldName) => void;
	onCredentialFocus: (field: CredentialFieldName) => void;
};

export const ClaudePlatformFields: FC<ClaudePlatformFieldsProps> = ({
	editing,
	authMode,
	awsExternalId,
	getFieldHelpers,
	onAuthModeChange,
	onRegionChange,
	onCredentialBlur,
	onCredentialFocus,
}) => {
	const modeLabelId = useId();
	return (
		<>
			<div className="grid grid-cols-2 items-start gap-4">
				<FormField
					required
					field={getFieldHelpers("claudePlatformRegion")}
					label="Region"
					description="AWS region serving the workspace. It also scopes the request signature."
					className="w-full"
					placeholder={CLAUDE_PLATFORM_DEFAULT_REGION}
					onChange={(event) => onRegionChange(event.target.value)}
				/>
				<FormField
					required
					field={getFieldHelpers("claudePlatformWorkspaceId")}
					label="Workspace ID"
					description="Anthropic workspace that every request is billed to."
					className="w-full"
					placeholder="wrkspc_..."
				/>
			</div>
			<FormField
				required
				field={getFieldHelpers("baseUrl", { backendFieldName: "base_url" })}
				label="Endpoint"
				description={
					<>
						Defaults to{" "}
						<code>https://aws-external-anthropic.{"{region}"}.api.aws</code>.
						Override it only to route through a proxy; the region above still
						decides the signing scope.
					</>
				}
				className="w-full"
				placeholder={claudePlatformBaseUrl(CLAUDE_PLATFORM_DEFAULT_REGION)}
			/>
			<div className="flex flex-col gap-2">
				<Label id={modeLabelId}>AWS authentication</Label>
				<RadioGroup
					className="gap-3"
					aria-labelledby={modeLabelId}
					value={authMode}
					onValueChange={(next) =>
						onAuthModeChange(next as AIProviderClaudePlatformAWSAuthMode)
					}
				>
					<RadioOption
						id={`${modeLabelId}-iam`}
						value="iam"
						label="AWS IAM"
						description="Sign requests with AWS credentials. No API key is stored."
					/>
					<RadioOption
						id={`${modeLabelId}-api-key`}
						value="api_key"
						label="Workspace API key"
						description="Authenticate with a key issued for the Anthropic workspace."
					/>
				</RadioGroup>
			</div>
			{authMode === "iam" ? (
				<>
					<div className="grid grid-cols-2 items-start gap-4">
						<CredentialField
							label="Access key"
							helpers={getFieldHelpers("accessKey")}
							onBlur={() => onCredentialBlur("accessKey")}
							onFocus={() => onCredentialFocus("accessKey")}
							autoComplete="new-password"
						/>
						<CredentialField
							label="Access key secret"
							helpers={getFieldHelpers("accessKeySecret")}
							onBlur={() => onCredentialBlur("accessKeySecret")}
							onFocus={() => onCredentialFocus("accessKeySecret")}
							autoComplete="new-password"
						/>
					</div>
					<p className="text-xs text-content-secondary m-0">
						Optional. Leave both fields blank to authenticate with the AWS
						environment (IAM role, instance profile, AWS_PROFILE).
					</p>
					<FormField
						field={getFieldHelpers("roleArn")}
						label="Role ARN"
						description="Optional. When set, the gateway assumes that role (using the base identity) before signing."
						className="w-full"
						placeholder="arn:aws:iam::123456789012:role/ClaudePlatformRole"
					/>
					{editing && awsExternalId && (
						<div className="flex flex-col gap-2">
							<Label>External ID</Label>
							<CodeExample secret={false} code={awsExternalId} />
							<p className="text-xs text-content-secondary m-0">
								Server-generated. Add it to the assumed role's trust policy as
								an <code>sts:ExternalId</code> condition so only this deployment
								can assume the role.
							</p>
						</div>
					)}
				</>
			) : (
				<CredentialField
					required
					label="Workspace API key"
					helpers={getFieldHelpers("apiKey")}
					onBlur={() => onCredentialBlur("apiKey")}
					onFocus={() => onCredentialFocus("apiKey")}
					autoComplete="new-password"
					placeholder="sk-ant-..."
				/>
			)}
		</>
	);
};
