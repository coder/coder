import {
	mcpServerConfigs,
	createMCPServerConfig as createMCPServerConfigMutation,
	updateMCPServerConfig as updateMCPServerConfigMutation,
	deleteMCPServerConfig as deleteMCPServerConfigMutation,
} from "api/queries/chats";
import type * as TypesGen from "api/typesGenerated";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { Button } from "components/Button/Button";
import { ExternalImage } from "components/ExternalImage/ExternalImage";
import { IconField } from "components/IconField/IconField";
import { Input } from "components/Input/Input";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "components/Select/Select";
import { Spinner } from "components/Spinner/Spinner";
import { Switch } from "components/Switch/Switch";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "components/Tooltip/Tooltip";
import {
	CheckCircleIcon,
	ChevronLeftIcon,
	ChevronRightIcon,
	CircleIcon,
	PlusIcon,
	ServerIcon,
	XIcon,
} from "lucide-react";
import {
	type FC,
	type FormEvent,
	type ReactNode,
	useCallback,
	useId,
	useMemo,
	useState,
} from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { cn } from "utils/cn";
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

// ── Shared field wrapper (matches ProviderForm pattern) ────────

interface FieldProps {
	label: string;
	htmlFor?: string;
	required?: boolean;
	description?: string;
	children: ReactNode;
}

const Field: FC<FieldProps> = ({
	label,
	htmlFor,
	required,
	description,
	children,
}) => (
	<div className="grid gap-1.5">
		<div className="flex items-baseline gap-1.5">
			<label
				htmlFor={htmlFor}
				className="text-sm font-medium text-content-primary"
			>
				{label}
			</label>
			{required && (
				<span className="text-xs font-bold text-content-destructive">*</span>
			)}
		</div>
		{description && (
			<p className="m-0 text-xs text-content-secondary">{description}</p>
		)}
		{children}
	</div>
);

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
							i > 0 &&
								"border-0 border-t border-solid border-border/50",
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
								{server.url}
								{" · "}
								{authTypeLabel(server.auth_type)}
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

	// ── Local state ─────────────────────────────────────────
	const [displayName, setDisplayName] = useState(
		server?.display_name ?? "",
	);
	const [slug, setSlug] = useState(server?.slug ?? "");
	const [slugTouched, setSlugTouched] = useState(false);
	const [description, setDescription] = useState(
		server?.description ?? "",
	);
	const [iconURL, setIconURL] = useState(server?.icon_url ?? "");
	const [url, setURL] = useState(server?.url ?? "");
	const [transport, setTransport] = useState(
		server?.transport ?? "streamable_http",
	);
	const [authType, setAuthType] = useState(server?.auth_type ?? "none");
	const [oauth2ClientID, setOauth2ClientID] = useState(
		server?.oauth2_client_id ?? "",
	);
	const [oauth2ClientSecret, setOauth2ClientSecret] = useState(
		server?.has_oauth2_secret ? SECRET_PLACEHOLDER : "",
	);
	const [oauth2SecretTouched, setOauth2SecretTouched] = useState(false);
	const [oauth2AuthURL, setOauth2AuthURL] = useState(
		server?.oauth2_auth_url ?? "",
	);
	const [oauth2TokenURL, setOauth2TokenURL] = useState(
		server?.oauth2_token_url ?? "",
	);
	const [oauth2Scopes, setOauth2Scopes] = useState(
		server?.oauth2_scopes ?? "",
	);
	const [apiKeyHeader, setApiKeyHeader] = useState(
		server?.api_key_header ?? "",
	);
	const [apiKeyValue, setApiKeyValue] = useState(
		server?.has_api_key ? SECRET_PLACEHOLDER : "",
	);
	const [apiKeyTouched, setApiKeyTouched] = useState(false);
	const [availability, setAvailability] = useState(
		server?.availability ?? "default_off",
	);
	const [enabled, setEnabled] = useState(server?.enabled ?? true);
	const [toolAllowList, setToolAllowList] = useState(
		joinList(server?.tool_allow_list),
	);
	const [toolDenyList, setToolDenyList] = useState(
		joinList(server?.tool_deny_list),
	);
	const [customHeaders, setCustomHeaders] = useState<
		Array<{ key: string; value: string }>
	>([]);
	const [customHeadersTouched, setCustomHeadersTouched] = useState(false);
	const [confirmingDelete, setConfirmingDelete] = useState(false);

	const handleDisplayNameChange = useCallback(
		(value: string) => {
			setDisplayName(value);
			if (!slugTouched) {
				setSlug(slugify(value));
			}
		},
		[slugTouched],
	);

	const handleSlugChange = useCallback((value: string) => {
		setSlugTouched(true);
		setSlug(value);
	}, []);

	const handleAddCustomHeader = useCallback(() => {
		setCustomHeadersTouched(true);
		setCustomHeaders((prev) => [...prev, { key: "", value: "" }]);
	}, []);

	const handleRemoveCustomHeader = useCallback((index: number) => {
		setCustomHeadersTouched(true);
		setCustomHeaders((prev) => prev.filter((_, i) => i !== index));
	}, []);

	const handleUpdateCustomHeader = useCallback(
		(index: number, field: "key" | "value", val: string) => {
			setCustomHeadersTouched(true);
			setCustomHeaders((prev) =>
				prev.map((h, i) => (i === index ? { ...h, [field]: val } : h)),
			);
		},
		[],
	);

	const handleSubmit = useCallback(
		async (e: FormEvent) => {
			e.preventDefault();

			const effectiveOAuth2Secret =
				oauth2SecretTouched &&
				oauth2ClientSecret !== SECRET_PLACEHOLDER
					? oauth2ClientSecret
					: undefined;
			const effectiveApiKeyValue =
				apiKeyTouched && apiKeyValue !== SECRET_PLACEHOLDER
					? apiKeyValue
					: undefined;

			const req: TypesGen.CreateMCPServerConfigRequest = {
				display_name: displayName.trim(),
				slug: slug.trim(),
				description: description.trim(),
				icon_url: iconURL.trim(),
				url: url.trim(),
				transport,
				auth_type: authType,
				availability,
				enabled,
				...(authType === "oauth2" && {
					oauth2_client_id: oauth2ClientID.trim(),
					oauth2_client_secret: effectiveOAuth2Secret,
					oauth2_auth_url: oauth2AuthURL.trim() || undefined,
					oauth2_token_url: oauth2TokenURL.trim() || undefined,
					oauth2_scopes: oauth2Scopes.trim() || undefined,
				}),
				...(authType === "api_key" && {
					api_key_header: apiKeyHeader.trim() || undefined,
					api_key_value: effectiveApiKeyValue,
				}),
				...(authType === "custom_headers" &&
					customHeadersTouched && {
						custom_headers: Object.fromEntries(
							customHeaders
								.filter((h) => h.key.trim() !== "")
								.map((h) => [h.key.trim(), h.value]),
						),
					}),
				tool_allow_list: splitList(toolAllowList),
				tool_deny_list: splitList(toolDenyList),
			};

			await onSave(req, server?.id);
		},
		[
			displayName,
			slug,
			description,
			iconURL,
			url,
			transport,
			authType,
			oauth2ClientID,
			oauth2ClientSecret,
			oauth2SecretTouched,
			oauth2AuthURL,
			oauth2TokenURL,
			oauth2Scopes,
			apiKeyHeader,
			apiKeyValue,
			apiKeyTouched,
			customHeaders,
			customHeadersTouched,
			availability,
			enabled,
			toolAllowList,
			toolDenyList,
			server,
			onSave,
		],
	);

	const isDisabled = isSaving || isDeleting;
	const canSubmit =
		displayName.trim() !== "" &&
		slug.trim() !== "" &&
		url.trim() !== "" &&
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
						iconUrl={iconURL}
						name={displayName || "New Server"}
						className="h-8 w-8"
					/>
					<input
						type="text"
						value={displayName}
						onChange={(e) => handleDisplayNameChange(e.target.value)}
						disabled={isDisabled}
						className="m-0 min-w-0 flex-1 border-0 bg-transparent p-0 text-lg font-medium text-content-primary outline-none placeholder:text-content-secondary focus:ring-0"
						placeholder="Server display name"
						aria-label="Display Name"
					/>
					<Tooltip>
						<TooltipTrigger asChild>
							<span className="ml-auto inline-flex">
								<Switch
									checked={enabled}
									onCheckedChange={setEnabled}
									aria-label="Enabled"
									disabled={isDisabled}
								/>
							</span>
						</TooltipTrigger>
						<TooltipContent side="bottom">
							{enabled ? "Disable" : "Enable"} this server
						</TooltipContent>
					</Tooltip>
				</div>
				<hr className="my-4 border-0 border-t border-solid border-border" />

				<form
					id={formId}
					onSubmit={(e) => void handleSubmit(e)}
					className="flex flex-1 flex-col"
					autoComplete="off"
				>
					<div className="space-y-5">							{/* ── Identity row: slug + description side by side ── */}
						<div className="grid items-start gap-5 sm:grid-cols-2">						<Field
							label="Slug"
							htmlFor={`${formId}-slug`}
							required
							description="URL-safe identifier."
						>
							<Input
								id={`${formId}-slug`}
								className="h-9 text-[13px]"
								value={slug}
								onChange={(e) =>
									handleSlugChange(e.target.value)
								}
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
								value={description}
								onChange={(e) =>
									setDescription(e.target.value)
								}
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
							value={iconURL}
							onChange={(e) => setIconURL(e.target.value)}
							onPickEmoji={(value) => setIconURL(value)}
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
								value={url}
								onChange={(e) => setURL(e.target.value)}
								placeholder="https://mcp.example.com/sse"
								disabled={isDisabled}
							/>
						</Field>

						<Field label="Transport" htmlFor={`${formId}-transport`}>
							<Select
								value={transport}
								onValueChange={setTransport}
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
										<SelectItem
											key={opt.value}
											value={opt.value}
										>
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
							value={authType}
							onValueChange={setAuthType}
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
									<SelectItem
										key={opt.value}
										value={opt.value}
									>
										{opt.label}
									</SelectItem>
								))}
							</SelectContent>
						</Select>
					</Field>

				{authType === "oauth2" && (
					<div className="space-y-4 rounded-lg border border-border bg-surface-secondary/30 p-4">
						<p className="m-0 text-xs text-content-secondary">
							Register a client with the external MCP server's OAuth2
							provider and enter the credentials below. Coder will
							handle the per-user authorization flow.
						</p>
						<div className="grid items-start gap-4 sm:grid-cols-2">								<Field
									label="Client ID"
									htmlFor={`${formId}-oauth-id`}
								>
									<Input
										id={`${formId}-oauth-id`}
										className="h-9 text-[13px]"
										value={oauth2ClientID}
										onChange={(e) =>
											setOauth2ClientID(e.target.value)
										}
										disabled={isDisabled}
									/>
								</Field>
								<Field
									label="Client Secret"
									htmlFor={`${formId}-oauth-secret`}
								>
										<Input
											id={`${formId}-oauth-secret`}
											className="h-9 font-mono text-[13px]"
											type="text"
											autoComplete="off"
											data-1p-ignore
											data-lpignore="true"
											data-form-type="other"
											data-bwignore
											style={{ WebkitTextSecurity: "disc" } as React.CSSProperties}
											value={oauth2ClientSecret}										onChange={(e) => {
											setOauth2SecretTouched(true);
											setOauth2ClientSecret(
												e.target.value,
											);
										}}
										onFocus={() => {
											if (
												!oauth2SecretTouched &&
												oauth2ClientSecret ===
													SECRET_PLACEHOLDER
											) {
												setOauth2ClientSecret("");
												setOauth2SecretTouched(true);
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
										value={oauth2AuthURL}
										onChange={(e) =>
											setOauth2AuthURL(e.target.value)
										}
										placeholder="https://provider.com/oauth2/authorize"
										disabled={isDisabled}
									/>
								</Field>
								<Field
									label="Token URL"
									htmlFor={`${formId}-oauth-token-url`}
								>
									<Input
										id={`${formId}-oauth-token-url`}
										className="h-9 text-[13px]"
										value={oauth2TokenURL}
										onChange={(e) =>
											setOauth2TokenURL(e.target.value)
										}
										placeholder="https://provider.com/oauth2/token"
										disabled={isDisabled}
									/>
								</Field>
							</div>
							<Field
								label="Scopes"
								htmlFor={`${formId}-oauth-scopes`}
							>
								<Input
									id={`${formId}-oauth-scopes`}
									className="h-9 text-[13px]"
									value={oauth2Scopes}
									onChange={(e) =>
										setOauth2Scopes(e.target.value)
									}
									placeholder="read write"
									disabled={isDisabled}
								/>
							</Field>
						</div>
					)}

					{authType === "api_key" && (
						<div className="grid items-start gap-4 rounded-lg border border-border bg-surface-secondary/30 p-4 sm:grid-cols-2">
							<Field
								label="Header Name"
								htmlFor={`${formId}-apikey-header`}
							>
								<Input
									id={`${formId}-apikey-header`}
									className="h-9 text-[13px]"
									value={apiKeyHeader}
									onChange={(e) =>
										setApiKeyHeader(e.target.value)
									}
									placeholder="Authorization"
									disabled={isDisabled}
								/>
							</Field>
							<Field
								label="API Key"
								htmlFor={`${formId}-apikey-value`}
							>
									<Input
										id={`${formId}-apikey-value`}
										className="h-9 font-mono text-[13px]"
										type="text"
										autoComplete="off"
										data-1p-ignore
										data-lpignore="true"
										data-form-type="other"
										data-bwignore
										style={{ WebkitTextSecurity: "disc" } as React.CSSProperties}
										value={apiKeyValue}									onChange={(e) => {
										setApiKeyTouched(true);
										setApiKeyValue(e.target.value);
									}}
									onFocus={() => {
										if (
											!apiKeyTouched &&
											apiKeyValue === SECRET_PLACEHOLDER
										) {
											setApiKeyValue("");
											setApiKeyTouched(true);
										}
									}}
									disabled={isDisabled}
								/>
							</Field>
						</div>
					)}

				{authType === "custom_headers" && (
					<div className="space-y-3 rounded-lg border border-border bg-surface-secondary/30 p-4">
						{server?.has_custom_headers && !customHeadersTouched && (
							<p className="m-0 text-xs text-content-secondary">
								This server has custom headers configured.
								Add headers below to replace them.
							</p>
						)}
						{customHeaders.map((header, index) => (
							<div key={index} className="flex items-start gap-2">
								<div className="grid flex-1 items-start gap-2 sm:grid-cols-2">
									<Input
										className="h-9 text-[13px]"
										value={header.key}
										onChange={(e) =>
											handleUpdateCustomHeader(
												index,
												"key",
												e.target.value,
											)
										}
										placeholder="Header name"
										disabled={isDisabled}
										aria-label={`Header ${index + 1} name`}
									/>
										<Input
											className="h-9 font-mono text-[13px]"
											type="text"
											autoComplete="off"
											data-1p-ignore
											data-lpignore="true"
											data-form-type="other"
											data-bwignore
											style={{ WebkitTextSecurity: "disc" } as React.CSSProperties}
											value={header.value}										onChange={(e) =>
											handleUpdateCustomHeader(
												index,
												"value",
												e.target.value,
											)
										}
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
									onClick={() =>
										handleRemoveCustomHeader(index)
									}
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
							onClick={handleAddCustomHeader}
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
								value={availability}
								onValueChange={setAvailability}
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
										<SelectItem
											key={opt.value}
											value={opt.value}
										>
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
						</Field>					{/* ── Tool governance row ── */}
					<hr className="!my-2 border-0 border-t border-solid border-border" />

					<div className="grid items-start gap-5 sm:grid-cols-2">
						<Field
							label="Tool Allow List"
							htmlFor={`${formId}-allow-list`}
							description="Comma-separated. Empty = all allowed."
						>
							<Input
								id={`${formId}-allow-list`}
								className="h-9 text-[13px]"
								value={toolAllowList}
								onChange={(e) =>
									setToolAllowList(e.target.value)
								}
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
								value={toolDenyList}
								onChange={(e) =>
									setToolDenyList(e.target.value)
								}
								placeholder="tool3, tool4"
								disabled={isDisabled}
							/>
						</Field>
					</div>
				</div>

				{/* Footer — pushed to bottom, matches ProviderForm */}
				<div className="mt-auto pt-6">
					<hr className="mb-4 border-0 border-t border-solid border-border" />
					{confirmingDelete && server ? (
						<div className="flex items-center gap-3">
							<p className="m-0 flex-1 text-sm text-content-secondary">
								Are you sure? This action is irreversible.
							</p>
							<div className="flex shrink-0 items-center gap-2">
								<Button
									variant="outline"
									size="lg"
									type="button"
									onClick={() =>
										setConfirmingDelete(false)
									}
									disabled={isDisabled}
								>
									Cancel
								</Button>
								<Button
									variant="destructive"
									size="lg"
									type="button"
									disabled={isDisabled}
									onClick={() =>
										void onDelete(server.id)
									}
								>
									{isDeleting && (
										<Spinner
											className="h-4 w-4"
											loading
										/>
									)}
									Delete server
								</Button>
							</div>
						</div>
					) : (
						<div className="flex items-center justify-between">
							{isEditing ? (
								<Button
									variant="outline"
									size="lg"
									type="button"
									className="text-content-secondary hover:text-content-destructive hover:border-border-destructive"
									disabled={isDisabled}
									onClick={() =>
										setConfirmingDelete(true)
									}
								>
									Delete
								</Button>
							) : (
								<div />
							)}
							<Button
								size="lg"
								type="submit"
								disabled={!canSubmit}
							>
								{isSaving && (
									<Spinner
										className="h-4 w-4"
										loading
									/>
								)}
								{isEditing
									? "Save changes"
									: "Create server"}
							</Button>
						</div>
					)}
				</div>
			</form>
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

	const serversQuery = useQuery(mcpServerConfigs());

	const createMut = useMutation(
		createMCPServerConfigMutation(queryClient),
	);
	const updateMut = useMutation(
		updateMCPServerConfigMutation(queryClient),
	);
	const deleteMut = useMutation(
		deleteMCPServerConfigMutation(queryClient),
	);

	type View =
		| { mode: "list" }
		| { mode: "form"; server: TypesGen.MCPServerConfig | null };
	const [view, setView] = useState<View>({ mode: "list" });

	const servers = useMemo(
		() =>
			(serversQuery.data ?? [])
				.slice()
				.sort((a, b) =>
					a.display_name.localeCompare(b.display_name),
				),
		[serversQuery.data],
	);

	const handleSave = useCallback(
		async (
			req: TypesGen.CreateMCPServerConfigRequest,
			id?: string,
		) => {
			try {
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
					await updateMut.mutateAsync({ id, req: updateReq });
				} else {
					await createMut.mutateAsync(req);
				}
				setView({ mode: "list" });
			} catch {
				// Error surfaced via mutation error state.
			}
		},
		[createMut, updateMut],
	);

	const handleDelete = useCallback(
		async (id: string) => {
			try {
				await deleteMut.mutateAsync(id);
				setView({ mode: "list" });
			} catch {
				// Error surfaced via mutation error state.
			}
		},
		[deleteMut],
	);

	if (serversQuery.isLoading) {
		return (
			<div className="flex items-center gap-1.5 text-xs text-content-secondary">
				<Spinner className="h-4 w-4" loading />
				Loading
			</div>
		);
	}

	return (
		<div className="flex min-h-full flex-col space-y-3">
			{view.mode === "list" ? (
				<ServerList
					servers={servers}
					onSelect={(server) =>
						setView({ mode: "form", server })
					}
					onAdd={() => setView({ mode: "form", server: null })}
					sectionLabel={sectionLabel}
					sectionDescription={sectionDescription}
					sectionBadge={sectionBadge}
				/>
			) : (
				<ServerForm
					server={view.server}
					isSaving={
						createMut.isPending || updateMut.isPending
					}
					isDeleting={deleteMut.isPending}
					onSave={handleSave}
					onDelete={handleDelete}
					onBack={() => setView({ mode: "list" })}
				/>
			)}

			{serversQuery.isError && (
				<ErrorAlert error={serversQuery.error} />
			)}
			{createMut.error && <ErrorAlert error={createMut.error} />}
			{updateMut.error && <ErrorAlert error={updateMut.error} />}
			{deleteMut.error && <ErrorAlert error={deleteMut.error} />}
		</div>
	);
};
