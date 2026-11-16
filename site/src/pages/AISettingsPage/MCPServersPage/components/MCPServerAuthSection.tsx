import type { FormikContextType } from "formik";
import { PlusIcon, XIcon } from "lucide-react";
import { Button } from "#/components/Button/Button";
import { Input } from "#/components/Input/Input";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "#/components/Select/Select";
import { passwordManagerIgnoreProps } from "#/utils/formUtils";
import { Field } from "./MCPServerFormFieldPrimitives";
import {
	AUTH_TYPE_OPTIONS,
	type MCPServerFormValues,
	OAUTH2_CLIENT_REGISTRATION_OPTIONS,
	SECRET_PLACEHOLDER,
} from "./mcpServerFormLogic";

type MCPServerAuthFieldsProps = {
	form: FormikContextType<MCPServerFormValues>;
	formId: string;
	disabled: boolean;
};

type MCPServerAuthSectionProps = MCPServerAuthFieldsProps & {
	canSelectUserOIDC: boolean;
};

export const MCPServerAuthSection: React.FC<MCPServerAuthSectionProps> = ({
	form,
	formId,
	disabled,
	canSelectUserOIDC,
}) => {
	const authTypeOptions = AUTH_TYPE_OPTIONS.filter(
		(option) =>
			option.value !== "user_oidc" ||
			canSelectUserOIDC ||
			form.initialValues.authType === "user_oidc",
	);

	return (
		<>
			<Field
				label="Authentication method"
				htmlFor={`${formId}-auth`}
				className="max-w-md"
			>
				<Select
					value={form.values.authType}
					onValueChange={(value) => void form.setFieldValue("authType", value)}
					disabled={disabled}
				>
					<SelectTrigger id={`${formId}-auth`} className="shadow-none">
						<SelectValue />
					</SelectTrigger>
					<SelectContent>
						{authTypeOptions.map((option) => (
							<SelectItem key={option.value} value={option.value}>
								{option.label}
							</SelectItem>
						))}
					</SelectContent>
				</Select>
			</Field>
			{form.values.authType === "oauth2" && (
				<OAuth2Fields form={form} formId={formId} disabled={disabled} />
			)}
			{form.values.authType === "api_key" && (
				<APIKeyFields form={form} formId={formId} disabled={disabled} />
			)}
			{form.values.authType === "custom_headers" && (
				<CustomHeadersFields form={form} formId={formId} disabled={disabled} />
			)}
			{form.values.authType === "user_oidc" && (
				<p className="m-0 text-sm text-content-secondary">
					Coder will forward the user's OIDC identity to this MCP server.
				</p>
			)}
		</>
	);
};

const DISCOVERED_VALUE_DESCRIPTION =
	"Coder sets this when it registers the client. Enter a value to override it.";

const OAuth2Fields: React.FC<MCPServerAuthFieldsProps> = ({
	form,
	formId,
	disabled,
}) => {
	const isDynamic = form.values.oauth2ClientRegistration === "dynamic";
	const { initialValues } = form;
	const registeredClientID =
		initialValues.authType === "oauth2" &&
		initialValues.oauth2ClientRegistration === "dynamic"
			? initialValues.oauth2ClientID
			: "";

	return (
		<div className="space-y-5">
			<Field
				label="Client registration"
				htmlFor={`${formId}-oauth-registration`}
				className="max-w-md"
			>
				<Select
					value={form.values.oauth2ClientRegistration}
					onValueChange={(value) =>
						void form.setFieldValue("oauth2ClientRegistration", value)
					}
					disabled={disabled}
				>
					<SelectTrigger
						id={`${formId}-oauth-registration`}
						className="shadow-none"
					>
						<SelectValue />
					</SelectTrigger>
					<SelectContent>
						{OAUTH2_CLIENT_REGISTRATION_OPTIONS.map((option) => (
							<SelectItem key={option.value} value={option.value}>
								{option.label}
							</SelectItem>
						))}
					</SelectContent>
				</Select>
			</Field>
			{isDynamic ? (
				<>
					<p className="m-0 text-sm text-content-secondary">
						{registeredClientID
							? "Coder registered a client with the OAuth2 provider of the server URL. When you save a different server URL, Coder registers a new client."
							: "When you save, Coder discovers the OAuth2 provider of the server URL and registers a client."}{" "}
						Coder will handle the per-user authorization flow.
					</p>
					{registeredClientID && (
						<p className="m-0 text-sm text-content-secondary">
							Registered client ID:{" "}
							<code className="wrap-anywhere">{registeredClientID}</code>
						</p>
					)}
				</>
			) : (
				<>
					<p className="m-0 text-sm text-content-secondary">
						Register a client with the external MCP server's OAuth2 provider and
						enter the credentials below. Coder will handle the per-user
						authorization flow.
					</p>
					<div className="grid items-start gap-4 sm:grid-cols-2">
						<Field label="Client ID" htmlFor={`${formId}-oauth-id`} required>
							<Input
								id={`${formId}-oauth-id`}
								className="shadow-none"
								{...form.getFieldProps("oauth2ClientID")}
								disabled={disabled}
							/>
						</Field>
						<Field label="Client secret" htmlFor={`${formId}-oauth-secret`}>
							<SecretInput
								id={`${formId}-oauth-secret`}
								value={form.values.oauth2ClientSecret}
								touched={form.values.oauth2SecretTouched}
								onTouch={() =>
									void form.setFieldValue("oauth2SecretTouched", true)
								}
								onValueChange={(value) =>
									void form.setFieldValue("oauth2ClientSecret", value)
								}
								onReset={() =>
									void form.setFieldValue("oauth2SecretTouched", false)
								}
								disabled={disabled}
							/>
						</Field>
					</div>
					<div className="grid items-start gap-4 sm:grid-cols-2">
						<Field
							label="Authorization URL"
							htmlFor={`${formId}-oauth-auth-url`}
							required
						>
							<Input
								id={`${formId}-oauth-auth-url`}
								className="placeholder:text-content-disabled shadow-none"
								{...form.getFieldProps("oauth2AuthURL")}
								placeholder="https://"
								disabled={disabled}
							/>
						</Field>
						<Field
							label="Token URL"
							htmlFor={`${formId}-oauth-token-url`}
							required
						>
							<Input
								id={`${formId}-oauth-token-url`}
								className="placeholder:text-content-disabled shadow-none"
								{...form.getFieldProps("oauth2TokenURL")}
								disabled={disabled}
							/>
						</Field>
					</div>
				</>
			)}
			<div className="grid items-start gap-4 sm:grid-cols-2">
				<Field
					label="Revocation URL"
					htmlFor={`${formId}-oauth-revocation-url`}
					description={isDynamic ? DISCOVERED_VALUE_DESCRIPTION : undefined}
				>
					<Input
						id={`${formId}-oauth-revocation-url`}
						className="placeholder:text-content-disabled shadow-none"
						{...form.getFieldProps("oauth2RevocationURL")}
						placeholder="https://"
						disabled={disabled}
					/>
				</Field>
				<Field
					label="Scopes"
					htmlFor={`${formId}-oauth-scopes`}
					description={isDynamic ? DISCOVERED_VALUE_DESCRIPTION : undefined}
				>
					<Input
						id={`${formId}-oauth-scopes`}
						className="shadow-none"
						{...form.getFieldProps("oauth2Scopes")}
						disabled={disabled}
					/>
				</Field>
			</div>
			<div className="grid gap-4 sm:grid-cols-2">
				<Field
					label="Resource URL"
					htmlFor={`${formId}-oauth-resource`}
					description={`Sent to the OAuth2 provider as the RFC 8707 resource parameter. ${
						isDynamic
							? DISCOVERED_VALUE_DESCRIPTION
							: "When empty, none is sent."
					}`}
				>
					<Input
						id={`${formId}-oauth-resource`}
						className="placeholder:text-content-disabled shadow-none"
						{...form.getFieldProps("oauth2Resource")}
						placeholder="https://"
						disabled={disabled}
					/>
				</Field>
			</div>
		</div>
	);
};

const APIKeyFields: React.FC<MCPServerAuthFieldsProps> = ({
	form,
	formId,
	disabled,
}) => (
	<div className="space-y-3">
		<p className="m-0 text-sm text-content-secondary">
			The header value is sent as-is. Include any prefix the server expects, for
			example <code>Bearer &lt;token&gt;</code>.
		</p>
		<div className="grid items-start gap-4 sm:grid-cols-2">
			<Field label="Header name" htmlFor={`${formId}-api-header`} required>
				<Input
					id={`${formId}-api-header`}
					className="shadow-none"
					{...form.getFieldProps("apiKeyHeader")}
					placeholder="Authorization"
					disabled={disabled}
				/>
			</Field>
			<Field label="Header value" htmlFor={`${formId}-api-key`} required>
				<SecretInput
					id={`${formId}-api-key`}
					value={form.values.apiKeyValue}
					touched={form.values.apiKeyTouched}
					onTouch={() => void form.setFieldValue("apiKeyTouched", true)}
					onValueChange={(value) =>
						void form.setFieldValue("apiKeyValue", value)
					}
					onReset={() => void form.setFieldValue("apiKeyTouched", false)}
					disabled={disabled}
				/>
			</Field>
		</div>
	</div>
);

export const SecretInput: React.FC<{
	id: string;
	value: string;
	touched: boolean;
	onTouch: () => void;
	onValueChange: (value: string) => void;
	onReset: () => void;
	disabled: boolean;
}> = ({ id, value, touched, onTouch, onValueChange, onReset, disabled }) => (
	<Input
		id={id}
		className="font-mono shadow-none [-webkit-text-security:disc]"
		type="text"
		{...passwordManagerIgnoreProps}
		value={value}
		onChange={(event) => {
			onTouch();
			onValueChange(event.target.value);
		}}
		onFocus={() => {
			if (!touched && value !== "") {
				onValueChange("");
				onTouch();
			}
		}}
		onBlur={() => {
			if (touched && value === "") {
				onValueChange(SECRET_PLACEHOLDER);
				onReset();
			}
		}}
		disabled={disabled}
	/>
);

const CustomHeadersFields: React.FC<MCPServerAuthFieldsProps> = ({
	form,
	formId,
	disabled,
}) => {
	const headers =
		form.values.customHeaders.length > 0
			? form.values.customHeaders
			: [{ key: "", value: "" }];
	const setHeaders = (nextHeaders: Array<{ key: string; value: string }>) => {
		void form.setFieldValue("customHeadersTouched", true);
		void form.setFieldValue("customHeaders", nextHeaders);
	};

	return (
		<div className="space-y-3">
			<p className="m-0 text-sm text-content-secondary">
				Enter custom headers to send with each request. Saving replaces existing
				custom headers.
			</p>
			{headers.map((header, index) => (
				<div
					key={index.toString()}
					className="grid items-end gap-3 sm:grid-cols-[1fr_1fr_auto]"
				>
					<CustomHeaderInput
						formId={formId}
						header={header}
						index={index}
						headers={headers}
						setHeaders={setHeaders}
						disabled={disabled}
					/>
				</div>
			))}
			<Button
				type="button"
				variant="outline"
				onClick={() => setHeaders([...headers, { key: "", value: "" }])}
				disabled={disabled}
			>
				<PlusIcon />
				Add header
			</Button>
		</div>
	);
};

const CustomHeaderInput: React.FC<{
	formId: string;
	header: { key: string; value: string };
	index: number;
	headers: Array<{ key: string; value: string }>;
	setHeaders: (headers: Array<{ key: string; value: string }>) => void;
	disabled: boolean;
}> = ({ formId, header, index, headers, setHeaders, disabled }) => (
	<>
		<Field label="Header name" htmlFor={`${formId}-custom-header-${index}`}>
			<Input
				id={`${formId}-custom-header-${index}`}
				className="shadow-none"
				value={header.key}
				onChange={(event) => {
					const nextHeaders = [...headers];
					nextHeaders[index] = { ...header, key: event.target.value };
					setHeaders(nextHeaders);
				}}
				disabled={disabled}
			/>
		</Field>
		<Field label="Header value" htmlFor={`${formId}-custom-value-${index}`}>
			<Input
				id={`${formId}-custom-value-${index}`}
				className="shadow-none"
				value={header.value}
				onChange={(event) => {
					const nextHeaders = [...headers];
					nextHeaders[index] = { ...header, value: event.target.value };
					setHeaders(nextHeaders);
				}}
				disabled={disabled}
			/>
		</Field>
		<Button
			type="button"
			variant="subtle"
			size="icon"
			aria-label="Remove header"
			disabled={disabled || headers.length === 1}
			onClick={() =>
				setHeaders(headers.filter((_, headerIndex) => headerIndex !== index))
			}
		>
			<XIcon />
		</Button>
	</>
);
