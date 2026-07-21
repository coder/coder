import type { FormikContextType } from "formik";
import { PlusIcon, XIcon } from "lucide-react";
import type { FC } from "react";
import { Button } from "#/components/Button/Button";
import { Input } from "#/components/Input/Input";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "#/components/Select/Select";
import { Field } from "./MCPServerFormFieldPrimitives";
import {
	AUTH_TYPE_OPTIONS,
	type MCPServerFormValues,
	SECRET_PLACEHOLDER,
} from "./mcpServerFormLogic";

interface MCPServerAuthSectionProps {
	form: FormikContextType<MCPServerFormValues>;
	formId: string;
	disabled: boolean;
}

export const MCPServerAuthSection: FC<MCPServerAuthSectionProps> = ({
	form,
	formId,
	disabled,
}) => {
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
						{AUTH_TYPE_OPTIONS.map((option) => (
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

const OAuth2Fields: FC<MCPServerAuthSectionProps> = ({
	form,
	formId,
	disabled,
}) => (
	<div className="space-y-5">
		<p className="m-0 text-sm text-content-secondary">
			Register a client with the external MCP server's OAuth2 provider and enter
			the credentials below. Coder will handle the per-user authorization flow.
		</p>
		<div className="grid items-start gap-4 sm:grid-cols-2">
			<Field label="Client ID" htmlFor={`${formId}-oauth-id`}>
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
					onTouch={() => void form.setFieldValue("oauth2SecretTouched", true)}
					onValueChange={(value) =>
						void form.setFieldValue("oauth2ClientSecret", value)
					}
					onReset={() => void form.setFieldValue("oauth2SecretTouched", false)}
					disabled={disabled}
				/>
			</Field>
		</div>
		<div className="grid items-start gap-4 sm:grid-cols-2">
			<Field label="Authorization URL" htmlFor={`${formId}-oauth-auth-url`}>
				<Input
					id={`${formId}-oauth-auth-url`}
					className="placeholder:text-content-disabled shadow-none"
					{...form.getFieldProps("oauth2AuthURL")}
					placeholder="https://"
					disabled={disabled}
				/>
			</Field>
			<Field label="Token URL" htmlFor={`${formId}-oauth-token-url`}>
				<Input
					id={`${formId}-oauth-token-url`}
					className="placeholder:text-content-disabled shadow-none"
					{...form.getFieldProps("oauth2TokenURL")}
					disabled={disabled}
				/>
			</Field>
		</div>
		<div className="grid items-start gap-4 sm:grid-cols-2">
			<Field label="Revocation URL" htmlFor={`${formId}-oauth-revocation-url`}>
				<Input
					id={`${formId}-oauth-revocation-url`}
					className="placeholder:text-content-disabled shadow-none"
					{...form.getFieldProps("oauth2RevocationURL")}
					placeholder="https://"
					disabled={disabled}
				/>
			</Field>
			<Field label="Scopes" htmlFor={`${formId}-oauth-scopes`}>
				<Input
					id={`${formId}-oauth-scopes`}
					className="shadow-none"
					{...form.getFieldProps("oauth2Scopes")}
					disabled={disabled}
				/>
			</Field>
		</div>
	</div>
);

const APIKeyFields: FC<MCPServerAuthSectionProps> = ({
	form,
	formId,
	disabled,
}) => (
	<div className="grid items-start gap-4 sm:grid-cols-2">
		<Field label="Header" htmlFor={`${formId}-api-header`}>
			<Input
				id={`${formId}-api-header`}
				className="shadow-none"
				{...form.getFieldProps("apiKeyHeader")}
				placeholder="Authorization"
				disabled={disabled}
			/>
		</Field>
		<Field label="API key" htmlFor={`${formId}-api-key`}>
			<SecretInput
				id={`${formId}-api-key`}
				value={form.values.apiKeyValue}
				touched={form.values.apiKeyTouched}
				onTouch={() => void form.setFieldValue("apiKeyTouched", true)}
				onValueChange={(value) => void form.setFieldValue("apiKeyValue", value)}
				onReset={() => void form.setFieldValue("apiKeyTouched", false)}
				disabled={disabled}
			/>
		</Field>
	</div>
);

const SecretInput: FC<{
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
		autoComplete="off"
		data-1p-ignore
		data-lpignore="true"
		data-form-type="other"
		data-bwignore
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

const CustomHeadersFields: FC<MCPServerAuthSectionProps> = ({
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

const CustomHeaderInput: FC<{
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
