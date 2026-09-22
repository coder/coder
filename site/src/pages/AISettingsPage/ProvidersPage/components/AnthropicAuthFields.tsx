import { type FC, useId } from "react";
import { Badge } from "#/components/Badge/Badge";
import { FormField } from "#/components/FormField/FormField";
import { Label } from "#/components/Label/Label";
import { Link as DocsLink } from "#/components/Link/Link";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "#/components/Select/Select";
import { docs } from "#/utils/docs";
import type { FormHelpers } from "#/utils/formUtils";
import { CredentialField } from "./CredentialField";
import {
	CLAUDE_PLATFORM_DEFAULT_REGION,
	claudePlatformBaseUrl,
} from "./claudePlatform";

/** Claude Platform is a settings variant of the Anthropic provider. */
export type AnthropicAuthMethod = "api_key" | "claude_platform_aws";

type FieldHelpersGetter = (
	fieldName: string,
	options?: { backendFieldName?: string },
) => FormHelpers;

type AnthropicAuthMethodFieldProps = {
	value: AnthropicAuthMethod;
	disabled?: boolean;
	onChange: (method: AnthropicAuthMethod) => void;
};

export const AnthropicAuthMethodField: FC<AnthropicAuthMethodFieldProps> = ({
	value,
	disabled = false,
	onChange,
}) => {
	const id = useId();
	return (
		<div className="flex flex-col gap-2">
			<Label htmlFor={id}>Platform</Label>
			<Select
				value={value}
				disabled={disabled}
				onValueChange={(next) => {
					if (next === "api_key" || next === "claude_platform_aws")
						onChange(next);
				}}
			>
				<SelectTrigger id={id} className="w-full">
					<SelectValue />
				</SelectTrigger>
				<SelectContent>
					<SelectItem value="api_key">Anthropic API</SelectItem>
					<SelectItem value="claude_platform_aws">
						Claude Platform for AWS
						<Badge variant="default" className="ml-2">
							Experimental
						</Badge>
					</SelectItem>
				</SelectContent>
			</Select>
			{disabled && (
				<p className="text-xs text-content-secondary m-0">
					The platform is fixed after creation. Add a new provider to use
					another platform.
				</p>
			)}
		</div>
	);
};

type ClaudePlatformFieldsProps = {
	getFieldHelpers: FieldHelpersGetter;
	onRegionChange: (region: string) => void;
	onCredentialBlur: (field: "apiKey") => void;
	onCredentialFocus: (field: "apiKey") => void;
};

export const ClaudePlatformFields: FC<ClaudePlatformFieldsProps> = ({
	getFieldHelpers,
	onRegionChange,
	onCredentialBlur,
	onCredentialFocus,
}) => (
	<>
		<p className="text-sm text-content-secondary m-0">
			Claude Platform for AWS support is experimental and may change.{" "}
			<DocsLink
				size="sm"
				href={docs("/ai-coder/ai-gateway/providers#claude-platform-for-aws")}
				target="_blank"
				rel="noreferrer"
			>
				View docs
			</DocsLink>
		</p>
		<div className="grid grid-cols-2 items-start gap-4">
			<FormField
				required
				field={getFieldHelpers("claudePlatformRegion", {
					backendFieldName: "settings.region",
				})}
				label="Region"
				description="AWS region serving the workspace and scoping request signatures."
				className="w-full"
				placeholder={CLAUDE_PLATFORM_DEFAULT_REGION}
				onChange={(event) => onRegionChange(event.target.value)}
			/>
			<FormField
				required
				field={getFieldHelpers("claudePlatformWorkspaceId", {
					backendFieldName: "settings.workspace_id",
				})}
				label="Workspace ID"
				description="Claude Platform workspace used for every request."
				className="w-full"
				placeholder="wrkspc_..."
			/>
		</div>
		<FormField
			required
			field={getFieldHelpers("baseUrl", { backendFieldName: "base_url" })}
			label="Endpoint"
			description="Defaults to the regional Claude Platform endpoint. Override it to route through a proxy."
			className="w-full"
			placeholder={claudePlatformBaseUrl(CLAUDE_PLATFORM_DEFAULT_REGION)}
		/>
		<CredentialField
			label="Workspace API key"
			helpers={getFieldHelpers("apiKey")}
			onBlur={() => onCredentialBlur("apiKey")}
			onFocus={() => onCredentialFocus("apiKey")}
			autoComplete="new-password"
			placeholder="sk-ant-..."
		/>
		<p className="text-xs text-content-secondary m-0">
			Optional. Client keys take precedence when BYOK is enabled, followed by
			stored provider keys. Without either, requests use the gateway's ambient
			AWS credentials. AWS identity is configured on the gateway, not per
			provider.
		</p>
	</>
);
