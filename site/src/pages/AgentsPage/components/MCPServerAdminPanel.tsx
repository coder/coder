import { useFormik } from "formik";
import {
	CheckCircleIcon,
	ChevronLeftIcon,
	ChevronRightIcon,
	CircleIcon,
	PlusIcon,
	ServerIcon,
	XIcon,
} from "lucide-react";
import { type FC, type ReactNode, useId, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { useSearchParams } from "react-router";
import {
	createMCPServerConfig as createMCPServerConfigMutation,
	deleteMCPServerConfig as deleteMCPServerConfigMutation,
	mcpServerConfigs,
	updateMCPServerConfig as updateMCPServerConfigMutation,
} from "#/api/queries/chats";
import type * as TypesGen from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Button } from "#/components/Button/Button";
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from "#/components/Dialog/Dialog";
import { ExternalImage } from "#/components/ExternalImage/ExternalImage";
import { IconField } from "#/components/IconField/IconField";
import { Input } from "#/components/Input/Input";
import { Label } from "#/components/Label/Label";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "#/components/Select/Select";
import { Spinner } from "#/components/Spinner/Spinner";
import { Switch } from "#/components/Switch/Switch";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { cn } from "#/utils/cn";
import { ProviderField as Field } from "./ChatModelAdminPanel/ProviderForm";
import { SectionHeader } from "./SectionHeader";

// ── Constants ──────────────────────────────────────────────────

const SECRET_PLACEHOLDER = "••••••••••••••••";

const TRANSPORT_OPTIONS = [
	{ value: "streamable_http", label: "Streamable HTTP" },
	{ value: "sse", label: "SSE" },
] as const;

const AUTH_TYPE_OPTIONS = [
	{ value: "none", label: "None" },
	{ value: "oauth2", label: "OAuth2" },
	{ value: "api_key", label: "API Key" },
	{ value: "custom_headers", label: "Custom Headers" },
] as const;

const AVAILABILITY_OPTIONS = [
	{
		value: "force_on",
		label: "Force On",
		description: "Always injected into every chat session.",
	},
	{
		value: "default_on",
		label: "Default On",
		description: "Pre-selected but users can opt out.",
	},
	{
		value: "default_off",
		label: "Default Off",
		description: "Available but users must opt in.",
	},
] as const;

// ── Helpers ────────────────────────────────────────────────────

const slugify = (value: string): string =>
	value
		.toLowerCase()
		.trim()
		.replace(/[^a-z0-9-]+/g, "-")
		.replace(/^-+|-+$/g, "");

const splitList = (value: string): string[] =>
	value
		.split(",")
		.map((s) => s.trim())
		.filter(Boolean);

const joinList = (arr: readonly string[] | undefined): string =>
	arr?.join(", ") ?? "";

const authTypeLabel = (t: string) =>
	AUTH_TYPE_OPTIONS.find((o) => o.value === t)?.label ?? t;

// ── Server icon ────────────────────────────────────────────────

const MCPServerIcon: FC<{
	iconUrl: string;
	name: string;
	className?: string;
}> = ({ iconUrl, name, className }) => {
	if (iconUrl) {
		return (
			<div
				className={cn(
					"flex shrink-0 items-center justify-center rounded-full bg-surface-secondary",
					className,
				)}
			>
				<ExternalImage
					src={iconUrl}
					alt={`${name} icon`}
					className="h-3/5 w-3/5"
				/>
			</div>
		);
	}
	return (
		<div
			className={cn(
				"flex shrink-0 items-center justify-center rounded-full bg-surface-secondary",
				className,
			)}
		>
			<ServerIcon className="h-3/5 w-3/5 text-content-secondary" />
		</div>
	);
};

// ── Server List ────────────────────────────────────────────────

interface ServerListProps {
	servers: readonly TypesGen.MCPServerConfig[];
	onSelect: (server: TypesGen.MCPServerConfig) => void;
	onAdd: () => void;
	sectionLabel?: string;
	sectionDescription?: string;
	sectionBadge?: ReactNode;
}

const ServerList: FC<ServerListProps> = ({
	servers,
	onSelect,
	onAdd,
	sectionLabel,
	sectionDescription,
	sectionBadge,
}) => (
	<>
		<SectionHeader
			label={sectionLabel ?? "MCP Servers"}
			description={
				sectionDescription ??
				"Configure external MCP servers that provide additional tools for AI chat sessions."
			}
			badge={sectionBadge}
			action={
				<Button size="sm" onClick={onAdd}>
					<PlusIcon className="h-4 w-4" />
					Add Server
				</Button>
			}
		/>

		{servers.length === 0 ? (
			<div className="rounded-lg border border-dashed border-border bg-surface-primary p-6 text-center text-[13px] text-content-secondary">
				No MCP servers configured yet. Add a server to get started.
			</div>
		) : (
			<div>
				{servers.map((server, i) => (
					<button
						key={server.id}
						type="button"
						onClick={() => onSelect(server)}
						aria-label={`${server.display_name} (${server.enabled ? "enabled" : "disabled"})`}
						className={cn(
							"flex w-full cursor-pointer items-center gap-3.5 bg-transparent border-0 p-0 px-3 py-3 text-left transition-colors hover:bg-surface-secondary/30",
							i > 0 && "border-0 border-t border-solid border-border/50",
						)}
					>
						<MCPServerIcon
							iconUrl={server.icon_url}
							name={server.display_name}
							className="h-8 w-8"
						/>
						<div className="min-w-0 flex-1">
							<span className="block truncate text-[15px] font-medium text-content-primary text-left">
								{server.display_name}
							</span>
							<span className="block truncate text-xs text-content-secondary">
								{server.url} · {authTypeLabel(server.auth_type)}
							</span>
						</div>
						{server.enabled ? (
							<CheckCircleIcon className="h-4 w-4 shrink-0 text-content-success" />
						) : (
							<CircleIcon className="h-4 w-4 shrink-0 text-content-secondary opacity-40" />
						)}
						<ChevronRightIcon className="h-5 w-5 shrink-0 text-content-secondary" />
					</button>
				))}
			</div>
		)}
	</>
);

// ── Server Form ────────────────────────────────────────────────

interface MCPServerFormValues {
	displayName: string;
	slug: string;
	slugTouched: boolean;
	description: string;
	iconURL: string;
	url: string;
	transport: string;
	authType: string;
	oauth2ClientID: string;
	oauth2ClientSecret: string;
	oauth2SecretTouched: boolean;
	oauth2AuthURL: string;
	oauth2TokenURL: string;
	oauth2Scopes: string;
	apiKeyHeader: string;
	apiKeyValue: string;
	apiKeyTouched: boolean;
	availability: string;
	enabled: boolean;
	modelIntent: boolean;
	toolAllowList: string;
	toolDenyList: string;
	customHeaders: Array<{ key: string; value: string }>;
	customHeadersTouched: boolean;
}

const buildInitialValues = (
	server: TypesGen.MCPServerConfig | null,
): MCPServerFormValues => ({
	displayName: server?.display_name ?? "",
	slug: server?.slug ?? "",
	slugTouched: false,
	description: server?.description ?? "",
	iconURL: server?.icon_url ?? "",
	url: server?.url ?? "",
	transport: server?.transport ?? "streamable_http",
	authType: server?.auth_type ?? "none",
	oauth2ClientID: server?.oauth2_client_id ?? "",
	oauth2ClientSecret: server?.has_oauth2_secret ? SECRET_PLACEHOLDER : "",
	oauth2SecretTouched: false,
	oauth2AuthURL: server?.oauth2_auth_url ?? "",
	oauth2TokenURL: server?.oauth2_token_url ?? "",
	oauth2Scopes: server?.oauth2_scopes ?? "",
	apiKeyHeader: server?.api_key_header ?? "",
	apiKeyValue: server?.has_api_key ? SECRET_PLACEHOLDER : "",
	apiKeyTouched: false,
	availability: server?.availability ?? "default_off",
	enabled: server?.enabled ?? true,
	modelIntent: server?.model_intent ?? false,
	toolAllowList: joinList(server?.tool_allow_list),
	toolDenyList: joinList(server?.tool_deny_list),
	customHeaders: [],
	customHeadersTouched: false,
});

interface ServerFormProps {
	server: TypesGen.MCPServerConfig | null;
	isSaving: boolean;
	isDeleting: boolean;
	onSave: (
		req: TypesGen.CreateMCPServerConfigRequest,
		id?: string,
	) => Promise<unknown>;
	onDelete: (id: string) => Promise<void>;
	onBack: () => void;
}

const ServerForm: FC<ServerFormProps> = ({
	server,
	isSaving,
	isDeleting,
	onSave,
	onDelete,
	onBack,
}) => {
	const formId = useId();
	const isEditing = server !== null;
	const [confirmingDelete, setConfirmingDelete] = useState(false);

	const form = useFormik<MCPServerFormValues>({
		initialValues: buildInitialValues(server),
		onSubmit: async (values) => {
			const effectiveOAuth2Secret =
				values.oauth2SecretTouched &&
				values.oauth2ClientSecret !== SECRET_PLACEHOLDER
					? values.oauth2ClientSecret
					: undefined;
			const effectiveApiKeyValue =
				values.apiKeyTouched && values.apiKeyValue !== SECRET_PLACEHOLDER
					? values.apiKeyValue
					: undefined;

			const req: TypesGen.CreateMCPServerConfigRequest = {
				display_name: values.displayName.trim(),
				slug: values.slug.trim(),
				description: values.description.trim(),
				icon_url: values.iconURL.trim(),
				url: values.url.trim(),
				transport: values.transport,
				auth_type: values.authType,
				availability: values.availability,
				enabled: values.enabled,
				model_intent: values.modelIntent,
				...(values.authType === "oauth2" && {
					oauth2_client_id: values.oauth2ClientID.trim(),
					oauth2_client_secret: effectiveOAuth2Secret,
					oauth2_auth_url: values.oauth2AuthURL.trim() || undefined,
					oauth2_token_url: values.oauth2TokenURL.trim() || undefined,
					oauth2_scopes: values.oauth2Scopes.trim() || undefined,
				}),
				...(values.authType === "api_key" && {
					api_key_header: values.apiKeyHeader.trim() || undefined,
					api_key_value: effectiveApiKeyValue,
				}),
				...(values.authType === "custom_headers" &&
					values.customHeadersTouched && {
						custom_headers: Object.fromEntries(
							values.customHeaders
								.filter((h) => h.key.trim() !== "")
								.map((h) => [h.key.trim(), h.value]),
						),
					}),
				tool_allow_list: splitList(values.toolAllowList),
				tool_deny_list: splitList(values.toolDenyList),
			};

			await onSave(req, server?.id);
		},
	});

	const isDisabled = isSaving || isDeleting;
	const canSubmit =
		form.values.displayName.trim() !== "" &&
		form.values.slug.trim() !== "" &&
		form.values.url.trim() !== "" &&
		!isDisabled;

	return (
		<div className="flex min-h-full flex-col">
			{/* Back */}
			<button
				type="button"
				onClick={onBack}
				className="mb-4 inline-flex cursor-pointer items-center gap-0.5 bg-transparent border-0 p-0 text-sm text-content-secondary transition-colors hover:text-content-primary"
			>
				<ChevronLeftIcon className="h-4 w-4" />
				Back
			</button>
			{/* Header with icon + editable name + enabled toggle */}
			<div className="flex items-center gap-3">
				<MCPServerIcon
					iconUrl={form.values.iconURL}
					name={form.values.displayName || "New Server"}
					className="h-8 w-8"
				/>
				<input
					type="text"
					value={form.values.displayName}
					onChange={(e) => {
						form.setFieldValue("displayName", e.target.value);
						if (!form.values.slugTouched) {
							form.setFieldValue("slug", slugify(e.target.value));
						}
					}}
					disabled={isDisabled}
					className="m-0 min-w-0 flex-1 border-0 bg-transparent p-0 text-lg font-medium text-content-primary outline-none placeholder:text-content-secondary focus:ring-0"
					placeholder="Server display name"
					aria-label="Display Name"
				/>
				<Tooltip>
					<TooltipTrigger asChild>
						<span className="ml-auto inline-flex">
							<Switch
								checked={form.values.enabled}
								onCheckedChange={(v) => {
									form.setFieldValue("enabled", v);
								}}
								aria-label="Enabled"
								disabled={isDisabled}
							/>
						</span>
					</TooltipTrigger>
					<TooltipContent side="bottom">
						{form.values.enabled ? "Disable" : "Enable"} this server
					</TooltipContent>
				</Tooltip>
			</div>
			<hr className="my-4 border-0 border-t border-solid border-border" />
			<form
				id={formId}
				onSubmit={form.handleSubmit}
				className="flex flex-1 flex-col"
				autoComplete="off"
			>
				<div className="space-y-5">
					{" "}
					{/* ── Identity row: slug + description side by side ── */}
					<div className="grid items-start gap-5 sm:grid-cols-2">
						{" "}
						<Field
							label="Slug"
							htmlFor={`${formId}-slug`}
							required
							description="URL-safe identifier."
						>
							<Input
								id={`${formId}-slug`}
								className="h-9 text-[13px]"
								value={form.values.slug}
								onChange={(e) => {
									form.setFieldValue("slugTouched", true);
									form.setFieldValue("slug", e.target.value);
								}}
								placeholder="e.g. sentry"
								disabled={isDisabled}
							/>
						</Field>
						<Field
							label="Description"
							htmlFor={`${formId}-desc`}
							description="Brief summary of what this server provides."
						>
							<Input
								id={`${formId}-desc`}
								className="h-9 text-[13px]"
								{...form.getFieldProps("description")}
								placeholder="Optional description"
								disabled={isDisabled}
							/>
						</Field>
					</div>
					<Field
						label="Icon"
						description="Pick an emoji or paste an image URL."
					>
						<IconField
							value={form.values.iconURL}
							onChange={(e) => {
								form.setFieldValue("iconURL", e.target.value);
							}}
							onPickEmoji={(value) => {
								form.setFieldValue("iconURL", value);
							}}
							disabled={isDisabled}
						/>
					</Field>
					{/* ── Connection row: URL + transport side by side ── */}
					<hr className="!my-2 border-0 border-t border-solid border-border" />
					<div className="grid items-start gap-5 sm:grid-cols-[1fr_auto]">
						<Field
							label="Server URL"
							htmlFor={`${formId}-url`}
							required
							description="The endpoint URL for this MCP server."
						>
							<Input
								id={`${formId}-url`}
								className="h-9 text-[13px]"
								{...form.getFieldProps("url")}
								placeholder="https://mcp.example.com/sse"
								disabled={isDisabled}
							/>
						</Field>

						<Field label="Transport" htmlFor={`${formId}-transport`}>
							<Select
								value={form.values.transport}
								onValueChange={(v) => {
									form.setFieldValue("transport", v);
								}}
								disabled={isDisabled}
							>
								<SelectTrigger
									id={`${formId}-transport`}
									className="h-9 min-w-[160px] text-[13px]"
								>
									<SelectValue />
								</SelectTrigger>
								<SelectContent>
									{TRANSPORT_OPTIONS.map((opt) => (
										<SelectItem key={opt.value} value={opt.value}>
											{opt.label}
										</SelectItem>
									))}
								</SelectContent>
							</Select>
						</Field>
					</div>
					{/* ── Authentication ── */}
					<hr className="!my-2 border-0 border-t border-solid border-border" />
					<Field
						label="Authentication"
						htmlFor={`${formId}-auth`}
						description="How users authenticate with this MCP server."
					>
						<Select
							value={form.values.authType}
							onValueChange={(v) => {
								form.setFieldValue("authType", v);
							}}
							disabled={isDisabled}
						>
							<SelectTrigger
								id={`${formId}-auth`}
								className="h-9 max-w-[240px] text-[13px]"
							>
								<SelectValue />
							</SelectTrigger>
							<SelectContent>
								{AUTH_TYPE_OPTIONS.map((opt) => (
									<SelectItem key={opt.value} value={opt.value}>
										{opt.label}
									</SelectItem>
								))}
							</SelectContent>
						</Select>
					</Field>
					{form.values.authType === "oauth2" && (
						<div className="space-y-4 rounded-lg border border-border bg-surface-secondary/30 p-4">
							<p className="m-0 text-xs text-content-secondary">
								Register a client with the external MCP server's OAuth2 provider
								and enter the credentials below. Coder will handle the per-user
								authorization flow.
							</p>
							<div className="grid items-start gap-4 sm:grid-cols-2">
								{" "}
								<Field label="Client ID" htmlFor={`${formId}-oauth-id`}>
									<Input
										id={`${formId}-oauth-id`}
										className="h-9 text-[13px]"
										{...form.getFieldProps("oauth2ClientID")}
										disabled={isDisabled}
									/>
								</Field>
								<Field label="Client Secret" htmlFor={`${formId}-oauth-secret`}>
									<Input
										id={`${formId}-oauth-secret`}
										className="h-9 font-mono text-[13px] [-webkit-text-security:disc]"
										type="text"
										autoComplete="off"
										data-1p-ignore
										data-lpignore="true"
										data-form-type="other"
										data-bwignore
										value={form.values.oauth2ClientSecret}
										onChange={(e) => {
											form.setFieldValue("oauth2SecretTouched", true);
											form.setFieldValue("oauth2ClientSecret", e.target.value);
										}}
										onFocus={() => {
											if (
												!form.values.oauth2SecretTouched &&
												form.values.oauth2ClientSecret === SECRET_PLACEHOLDER
											) {
												form.setFieldValue("oauth2ClientSecret", "");
												form.setFieldValue("oauth2SecretTouched", true);
											}
										}}
										disabled={isDisabled}
									/>
								</Field>
							</div>
							<div className="grid items-start gap-4 sm:grid-cols-2">
								<Field
									label="Authorization URL"
									htmlFor={`${formId}-oauth-auth-url`}
								>
									<Input
										id={`${formId}-oauth-auth-url`}
										className="h-9 text-[13px]"
										{...form.getFieldProps("oauth2AuthURL")}
										placeholder="https://provider.com/oauth2/authorize"
										disabled={isDisabled}
									/>
								</Field>
								<Field label="Token URL" htmlFor={`${formId}-oauth-token-url`}>
									<Input
										id={`${formId}-oauth-token-url`}
										className="h-9 text-[13px]"
										{...form.getFieldProps("oauth2TokenURL")}
										placeholder="https://provider.com/oauth2/token"
										disabled={isDisabled}
									/>
								</Field>
							</div>
							<Field label="Scopes" htmlFor={`${formId}-oauth-scopes`}>
								<Input
									id={`${formId}-oauth-scopes`}
									className="h-9 text-[13px]"
									{...form.getFieldProps("oauth2Scopes")}
									placeholder="read write"
									disabled={isDisabled}
								/>
							</Field>
						</div>
					)}
					{form.values.authType === "api_key" && (
						<div className="grid items-start gap-4 rounded-lg border border-border bg-surface-secondary/30 p-4 sm:grid-cols-2">
							<Field label="Header Name" htmlFor={`${formId}-apikey-header`}>
								<Input
									id={`${formId}-apikey-header`}
									className="h-9 text-[13px]"
									{...form.getFieldProps("apiKeyHeader")}
									placeholder="Authorization"
									disabled={isDisabled}
								/>
							</Field>
							<Field label="API Key" htmlFor={`${formId}-apikey-value`}>
								<Input
									id={`${formId}-apikey-value`}
									className="h-9 font-mono text-[13px] [-webkit-text-security:disc]"
									type="text"
									autoComplete="off"
									data-1p-ignore
									data-lpignore="true"
									data-form-type="other"
									data-bwignore
									value={form.values.apiKeyValue}
									onChange={(e) => {
										form.setFieldValue("apiKeyTouched", true);
										form.setFieldValue("apiKeyValue", e.target.value);
									}}
									onFocus={() => {
										if (
											!form.values.apiKeyTouched &&
											form.values.apiKeyValue === SECRET_PLACEHOLDER
										) {
											form.setFieldValue("apiKeyValue", "");
											form.setFieldValue("apiKeyTouched", true);
										}
									}}
									disabled={isDisabled}
								/>
							</Field>
						</div>
					)}
					{form.values.authType === "custom_headers" && (
						<div className="space-y-3 rounded-lg border border-border bg-surface-secondary/30 p-4">
							{server?.has_custom_headers &&
								!form.values.customHeadersTouched && (
									<p className="m-0 text-xs text-content-secondary">
										This server has custom headers configured. Add headers below
										to replace them.
									</p>
								)}
							{form.values.customHeaders.map((header, index) => (
								<div key={index} className="flex items-start gap-2">
									<div className="grid flex-1 items-start gap-2 sm:grid-cols-2">
										<Input
											className="h-9 text-[13px]"
											value={header.key}
											onChange={(e) => {
												form.setFieldValue("customHeadersTouched", true);
												const updated = [...form.values.customHeaders];
												updated[index] = {
													...updated[index],
													key: e.target.value,
												};
												form.setFieldValue("customHeaders", updated);
											}}
											placeholder="Header name"
											disabled={isDisabled}
											aria-label={`Header ${index + 1} name`}
										/>
										<Input
											className="h-9 font-mono text-[13px] [-webkit-text-security:disc]"
											type="text"
											autoComplete="off"
											data-1p-ignore
											data-lpignore="true"
											data-form-type="other"
											data-bwignore
											value={header.value}
											onChange={(e) => {
												form.setFieldValue("customHeadersTouched", true);
												const updated = [...form.values.customHeaders];
												updated[index] = {
													...updated[index],
													value: e.target.value,
												};
												form.setFieldValue("customHeaders", updated);
											}}
											placeholder="Header value"
											disabled={isDisabled}
											aria-label={`Header ${index + 1} value`}
										/>
									</div>
									<Button
										variant="outline"
										size="icon"
										type="button"
										className="mt-0 h-9 w-9 shrink-0"
										onClick={() => {
											form.setFieldValue("customHeadersTouched", true);
											form.setFieldValue(
												"customHeaders",
												form.values.customHeaders.filter((_, i) => i !== index),
											);
										}}
										disabled={isDisabled}
										aria-label={`Remove header ${index + 1}`}
									>
										<XIcon className="h-4 w-4" />
									</Button>
								</div>
							))}
							<Button
								variant="outline"
								size="sm"
								type="button"
								onClick={() => {
									form.setFieldValue("customHeadersTouched", true);
									form.setFieldValue("customHeaders", [
										...form.values.customHeaders,
										{ key: "", value: "" },
									]);
								}}
								disabled={isDisabled}
							>
								<PlusIcon className="h-4 w-4" />
								Add header
							</Button>
						</div>
					)}
					{/* ── Availability ── */}
					<hr className="!my-2 border-0 border-t border-solid border-border" />
					<Field
						label="Availability"
						htmlFor={`${formId}-availability`}
						description="Controls how this server appears in new chats."
					>
						<Select
							value={form.values.availability}
							onValueChange={(v) => {
								form.setFieldValue("availability", v);
							}}
							disabled={isDisabled}
						>
							<SelectTrigger
								id={`${formId}-availability`}
								className="h-9 text-[13px]"
							>
								<SelectValue />
							</SelectTrigger>
							<SelectContent>
								{AVAILABILITY_OPTIONS.map((opt) => (
									<SelectItem key={opt.value} value={opt.value}>
										<div>
											<span>{opt.label}</span>
											<span className="ml-1.5 text-content-secondary">
												— {opt.description}
											</span>
										</div>
									</SelectItem>
								))}
							</SelectContent>
						</Select>
					</Field>{" "}
					{/* ── Tool governance row ── */}
					<hr className="!my-2 border-0 border-t border-solid border-border" />
					<div className="flex items-center justify-between">
						<div>
							<Label htmlFor={`${formId}-model-intent`}>Model intent</Label>
							<p className="text-sm text-content-secondary">
								Require the model to describe each tool call's purpose in
								natural language, shown as a status label in the UI.
							</p>
						</div>
						<Switch
							id={`${formId}-model-intent`}
							checked={form.values.modelIntent}
							onCheckedChange={(v) => {
								form.setFieldValue("modelIntent", v);
							}}
							disabled={isDisabled}
						/>
					</div>
					<div className="grid items-start gap-5 sm:grid-cols-2">
						{" "}
						<Field
							label="Tool Allow List"
							htmlFor={`${formId}-allow-list`}
							description="Comma-separated. Empty = all allowed."
						>
							<Input
								id={`${formId}-allow-list`}
								className="h-9 text-[13px]"
								{...form.getFieldProps("toolAllowList")}
								placeholder="tool1, tool2"
								disabled={isDisabled}
							/>
						</Field>
						<Field
							label="Tool Deny List"
							htmlFor={`${formId}-deny-list`}
							description="Comma-separated names to block."
						>
							<Input
								id={`${formId}-deny-list`}
								className="h-9 text-[13px]"
								{...form.getFieldProps("toolDenyList")}
								placeholder="tool3, tool4"
								disabled={isDisabled}
							/>
						</Field>
					</div>
				</div>

				{/* Footer — pushed to bottom, matches ProviderForm */}
				<div className="mt-auto pt-6">
					<hr className="mb-4 border-0 border-t border-solid border-border" />
					<div className="flex items-center justify-between">
						{isEditing ? (
							<Button
								variant="outline"
								size="lg"
								type="button"
								className="text-content-secondary hover:text-content-destructive hover:border-border-destructive"
								disabled={isDisabled}
								onClick={() => setConfirmingDelete(true)}
							>
								Delete
							</Button>
						) : (
							<div />
						)}
						<Button size="lg" type="submit" disabled={!canSubmit}>
							{isSaving && <Spinner className="h-4 w-4" loading />}
							{isEditing ? "Save changes" : "Create server"}
						</Button>
					</div>
				</div>
			</form>
			{server && (
				<Dialog
					open={confirmingDelete}
					onOpenChange={(open) => !open && setConfirmingDelete(false)}
				>
					<DialogContent variant="destructive">
						<DialogHeader>
							<DialogTitle>Delete server</DialogTitle>
							<DialogDescription>
								Are you sure you want to delete this MCP server? This action is
								irreversible.
							</DialogDescription>
						</DialogHeader>
						<DialogFooter>
							<Button
								variant="outline"
								onClick={() => setConfirmingDelete(false)}
								disabled={isDisabled}
							>
								Cancel
							</Button>
							<Button
								variant="destructive"
								onClick={() => void onDelete(server.id)}
								disabled={isDisabled}
							>
								{isDeleting && <Spinner className="h-4 w-4" loading />}
								Delete server
							</Button>
						</DialogFooter>
					</DialogContent>
				</Dialog>
			)}{" "}
		</div>
	);
};

// ── Main Panel ─────────────────────────────────────────────────

interface MCPServerAdminPanelProps {
	sectionLabel?: string;
	sectionDescription?: string;
	sectionBadge?: ReactNode;
}

export const MCPServerAdminPanel: FC<MCPServerAdminPanelProps> = ({
	sectionLabel,
	sectionDescription,
	sectionBadge,
}) => {
	const queryClient = useQueryClient();
	const [searchParams, setSearchParams] = useSearchParams();
	const serverId = searchParams.get("server");

	const serversQuery = useQuery(mcpServerConfigs());

	const createMut = useMutation(createMCPServerConfigMutation(queryClient));
	const updateMut = useMutation(updateMCPServerConfigMutation(queryClient));
	const deleteMut = useMutation(deleteMCPServerConfigMutation(queryClient));

	const servers = (serversQuery.data ?? [])
		.slice()
		.sort((a, b) => a.display_name.localeCompare(b.display_name));

	const editingServer =
		serverId && serverId !== "new"
			? (servers.find((s) => s.id === serverId) ?? null)
			: null;
	const isFormView = serverId !== null;
	const isCreating = serverId === "new";

	const handleSave = async (
		req: TypesGen.CreateMCPServerConfigRequest,
		id?: string,
	) => {
		if (id) {
			const updateReq: TypesGen.UpdateMCPServerConfigRequest = {
				...req,
				tool_allow_list: req.tool_allow_list
					? [...req.tool_allow_list]
					: undefined,
				tool_deny_list: req.tool_deny_list
					? [...req.tool_deny_list]
					: undefined,
			};
			try {
				await updateMut.mutateAsync({ id, req: updateReq });
			} catch {
				// Error surfaced via mutation error state.
				return;
			}
		} else {
			try {
				await createMut.mutateAsync(req);
			} catch {
				// Error surfaced via mutation error state.
				return;
			}
		}
		setSearchParams({});
	};

	const handleDelete = async (id: string) => {
		try {
			await deleteMut.mutateAsync(id);
		} catch {
			// Error surfaced via mutation error state.
			return;
		}
		setSearchParams({});
	};

	if (serversQuery.isLoading) {
		return <Spinner loading className="h-4 w-4" />;
	}

	return (
		<div className="flex min-h-full flex-col space-y-3">
			{!isFormView ? (
				<ServerList
					servers={servers}
					onSelect={(server) => setSearchParams({ server: server.id })}
					onAdd={() => setSearchParams({ server: "new" })}
					sectionLabel={sectionLabel}
					sectionDescription={sectionDescription}
					sectionBadge={sectionBadge}
				/>
			) : (
				<ServerForm
					key={serverId}
					server={isCreating ? null : editingServer}
					isSaving={createMut.isPending || updateMut.isPending}
					isDeleting={deleteMut.isPending}
					onSave={handleSave}
					onDelete={handleDelete}
					onBack={() => setSearchParams({})}
				/>
			)}

			{serversQuery.isError && <ErrorAlert error={serversQuery.error} />}
			{createMut.error && <ErrorAlert error={createMut.error} />}
			{updateMut.error && <ErrorAlert error={updateMut.error} />}
			{deleteMut.error && <ErrorAlert error={deleteMut.error} />}
		</div>
	);
};
