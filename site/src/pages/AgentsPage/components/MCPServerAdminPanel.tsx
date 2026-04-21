import { useFormik } from "formik";
import {
	CheckCircleIcon,
	ChevronDownIcon,
	ChevronRightIcon,
	CircleIcon,
	PencilIcon,
	PlusIcon,
	ServerIcon,
	XIcon,
} from "lucide-react";
import {
	type FC,
	lazy,
	type ReactNode,
	Suspense,
	useId,
	useState,
} from "react";
import { useLocation, useNavigate, useSearchParams } from "react-router";

import type * as TypesGen from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { ChevronDownIcon as AnimatedChevronDownIcon } from "#/components/AnimatedIcons/ChevronDown";
import { Badge } from "#/components/Badge/Badge";
import { Button } from "#/components/Button/Button";
import {
	Collapsible,
	CollapsibleContent,
	CollapsibleTrigger,
} from "#/components/Collapsible/Collapsible";
import { ExternalImage } from "#/components/ExternalImage/ExternalImage";
import { Input } from "#/components/Input/Input";
import {
	InputGroup,
	InputGroupAddon,
	InputGroupInput,
} from "#/components/InputGroup/InputGroup";
import { Label } from "#/components/Label/Label";
import { Loader } from "#/components/Loader/Loader";
import {
	Popover,
	PopoverContent,
	PopoverTrigger,
} from "#/components/Popover/Popover";
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
import { BackButton } from "./BackButton";
import { ProviderField as Field } from "./ChatModelAdminPanel/ProviderForm";
import { ConfirmDeleteDialog } from "./ConfirmDeleteDialog";
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
		description: "Always injected into every conversation.",
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

// ── Icon picker ──────────────────────────────────────────────────

const EmojiPicker = lazy(() => import("#/components/IconField/EmojiPicker"));

interface IconPickerFieldProps {
	id?: string;
	value: string;
	placeholder?: string;
	disabled?: boolean;
	onChange: (value: string) => void;
	onPickEmoji: (value: string) => void;
}

const IconPickerField: FC<IconPickerFieldProps> = ({
	id,
	value,
	placeholder,
	disabled,
	onChange,
	onPickEmoji,
}) => {
	const [open, setOpen] = useState(false);
	const hasIcon = value !== "";

	return (
		<InputGroup className="h-9">
			<InputGroupInput
				id={id}
				value={value}
				onChange={(e) => onChange(e.target.value)}
				placeholder={placeholder}
				disabled={disabled}
				className="h-9 min-w-0 text-[13px] placeholder:text-content-disabled"
				spellCheck={false}
			/>
			<InputGroupAddon align="inline-end" className="gap-1.5">
				{hasIcon && (
					<span className="flex h-5 w-5 items-center justify-center [&_img]:max-w-full [&_img]:object-contain">
						<ExternalImage
							alt=""
							src={value}
							// Hide the broken-image glyph while the URL is incomplete.
							onError={(e) => {
								e.currentTarget.style.display = "none";
							}}
							onLoad={(e) => {
								e.currentTarget.style.display = "inline";
							}}
						/>
					</span>
				)}
				<Popover open={open} onOpenChange={setOpen}>
					<PopoverTrigger asChild>
						<Button
							type="button"
							variant="subtle"
							size="sm"
							className="group h-7 gap-1"
							disabled={disabled}
							aria-label="Pick an emoji or icon"
						>
							Emoji
							<AnimatedChevronDownIcon />
						</Button>
					</PopoverTrigger>
					<PopoverContent side="bottom" align="end" className="w-min">
						<Suspense fallback={<Loader />}>
							<EmojiPicker
								onEmojiSelect={(emoji) => {
									const picked = emoji.src ?? `/emojis/${emoji.unified}.png`;
									onPickEmoji(picked);
									setOpen(false);
								}}
							/>
						</Suspense>
					</PopoverContent>
				</Popover>
			</InputGroupAddon>
		</InputGroup>
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
}) => {
	return (
		<>
			<SectionHeader
				label={sectionLabel ?? "MCP Servers"}
				description={
					sectionDescription ??
					"Configure external MCP servers that provide additional tools for Coder Agents."
				}
				badge={sectionBadge}
				action={
					<Button size="sm" onClick={onAdd}>
						<PlusIcon className="h-4 w-4" />
						Add server
					</Button>
				}
			/>

			{servers.length === 0 ? (
				<div className="flex flex-col items-center justify-center gap-3 px-6 py-12 text-center">
					<p className="m-0 text-sm text-content-secondary">
						No MCP servers configured yet.
					</p>
					<Button size="sm" onClick={onAdd} aria-label="Add your first server">
						<PlusIcon className="h-4 w-4" />
						Add your first server
					</Button>
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
								className="h-8 w-8 shrink-0"
							/>
							<div className="min-w-0 flex-1">
								<span
									className={cn(
										"block truncate text-[15px] font-medium text-left",
										server.enabled
											? "text-content-primary"
											: "text-content-secondary",
									)}
								>
									{server.display_name}
								</span>
								<span className="block truncate text-xs text-content-secondary">
									{server.url} · {authTypeLabel(server.auth_type)}
								</span>
							</div>
							{!server.enabled && (
								<Badge size="xs" variant="warning">
									disabled
								</Badge>
							)}
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
};

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
	allowInPlanMode: boolean;
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
	allowInPlanMode: server?.allow_in_plan_mode ?? false,
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
	const [showDetails, setShowDetails] = useState(false);
	const [showAuth, setShowAuth] = useState(false);
	const [showBehavior, setShowBehavior] = useState(false);

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
				allow_in_plan_mode: values.allowInPlanMode,
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
			<BackButton onClick={onBack} />
			{/* Header with icon + editable name + enabled toggle */}
			<div className="flex items-center gap-3">
				<MCPServerIcon
					iconUrl={form.values.iconURL}
					name={form.values.displayName || "New server"}
					className="h-8 w-8"
				/>
				<div className="inline-flex items-center gap-1">
					<div className="relative inline-grid">
						<span
							className="invisible col-start-1 row-start-1 whitespace-pre text-lg font-medium"
							aria-hidden="true"
						>
							{form.values.displayName || "Server display name"}
						</span>
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
							spellCheck={false}
							className="col-start-1 row-start-1 m-0 min-w-0 border-0 bg-transparent p-0 text-lg font-medium text-content-primary outline-none placeholder:text-content-secondary focus:ring-0"
							placeholder="Server display name"
							aria-label="Display Name"
						/>
					</div>
					<PencilIcon className="h-3.5 w-3.5 shrink-0 text-content-secondary" />
				</div>
				{isEditing && (
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
							{form.values.enabled
								? "Disable this server. It will be hidden from agents."
								: "Enable this server. It will be visible to agents."}
						</TooltipContent>
					</Tooltip>
				)}
			</div>
			<hr className="my-4 border-0 border-t border-solid border-border" />
			<form
				id={formId}
				onSubmit={form.handleSubmit}
				className="flex flex-1 flex-col"
				spellCheck={false}
				autoComplete="off"
			>
				<div className="space-y-6">
					<div className="space-y-4">
						<Field label="Slug" htmlFor={`${formId}-slug`} required>
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
						<div className="grid items-start gap-4 sm:grid-cols-[1fr_auto]">
							<Field label="Server URL" htmlFor={`${formId}-url`} required>
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
					</div>

					{/* ── Details section ── */}
					<Collapsible open={showDetails} onOpenChange={setShowDetails}>
						<div className="border-0 border-t border-solid border-border pt-4">
							<CollapsibleTrigger asChild>
								<button
									type="button"
									className="flex w-full cursor-pointer items-start justify-between border-0 bg-transparent p-0 text-left transition-colors hover:text-content-primary"
								>
									<div>
										<h3 className="m-0 text-sm font-medium text-content-primary">
											Details
										</h3>
										<p className="m-0 text-xs text-content-secondary">
											Optional description and icon shown to users.
										</p>
									</div>
									{showDetails ? (
										<ChevronDownIcon className="mt-0.5 h-4 w-4 shrink-0 text-content-secondary" />
									) : (
										<ChevronRightIcon className="mt-0.5 h-4 w-4 shrink-0 text-content-secondary" />
									)}
								</button>
							</CollapsibleTrigger>
							<CollapsibleContent className="space-y-4 pt-3">
								<Field label="Description" htmlFor={`${formId}-desc`}>
									<Input
										id={`${formId}-desc`}
										className="h-9 text-[13px]"
										{...form.getFieldProps("description")}
										placeholder="Optional description"
										disabled={isDisabled}
									/>
								</Field>
								<Field label="Icon">
									<IconPickerField
										value={form.values.iconURL}
										onChange={(v) => {
											form.setFieldValue("iconURL", v);
										}}
										onPickEmoji={(v) => {
											form.setFieldValue("iconURL", v);
										}}
										disabled={isDisabled}
									/>
								</Field>
							</CollapsibleContent>
						</div>
					</Collapsible>

					{/* ── Authentication section ── */}
					<Collapsible open={showAuth} onOpenChange={setShowAuth}>
						<div className="border-0 border-t border-solid border-border pt-4">
							<CollapsibleTrigger asChild>
								<button
									type="button"
									className="flex w-full cursor-pointer items-start justify-between border-0 bg-transparent p-0 text-left transition-colors hover:text-content-primary"
								>
									<div>
										<h3 className="m-0 text-sm font-medium text-content-primary">
											Authentication
										</h3>
										<p className="m-0 text-xs text-content-secondary">
											How users authenticate with this MCP server.
										</p>
									</div>
									{showAuth ? (
										<ChevronDownIcon className="mt-0.5 h-4 w-4 shrink-0 text-content-secondary" />
									) : (
										<ChevronRightIcon className="mt-0.5 h-4 w-4 shrink-0 text-content-secondary" />
									)}
								</button>
							</CollapsibleTrigger>
							<CollapsibleContent className="space-y-4 pt-3">
								<Field label="Authentication method" htmlFor={`${formId}-auth`}>
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
									<div className="space-y-4 rounded-lg border border-solid border-border/70 bg-surface-secondary/30 p-4">
										<p className="m-0 text-xs text-content-secondary">
											Register a client with the external MCP server's OAuth2
											provider and enter the credentials below. Coder will
											handle the per-user authorization flow.
										</p>
										<div className="grid items-start gap-4 sm:grid-cols-2">
											<Field label="Client ID" htmlFor={`${formId}-oauth-id`}>
												<Input
													id={`${formId}-oauth-id`}
													className="h-9 text-[13px]"
													{...form.getFieldProps("oauth2ClientID")}
													disabled={isDisabled}
												/>
											</Field>
											<Field
												label="Client Secret"
												htmlFor={`${formId}-oauth-secret`}
											>
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
														form.setFieldValue(
															"oauth2ClientSecret",
															e.target.value,
														);
													}}
													onFocus={() => {
														if (
															!form.values.oauth2SecretTouched &&
															form.values.oauth2ClientSecret ===
																SECRET_PLACEHOLDER
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
											<Field
												label="Token URL"
												htmlFor={`${formId}-oauth-token-url`}
											>
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
									<div className="grid items-start gap-4 rounded-lg border border-solid border-border/70 bg-surface-secondary/30 p-4 sm:grid-cols-2">
										<Field
											label="Header Name"
											htmlFor={`${formId}-apikey-header`}
										>
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
									<div className="space-y-3 rounded-lg border border-solid border-border/70 bg-surface-secondary/30 p-4">
										{server?.has_custom_headers &&
											!form.values.customHeadersTouched && (
												<p className="m-0 text-xs text-content-secondary">
													This server has custom headers configured. Add headers
													below to replace them.
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
															form.values.customHeaders.filter(
																(_, i) => i !== index,
															),
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
							</CollapsibleContent>
						</div>
					</Collapsible>
					{/* ── Behavior section ── */}
					<Collapsible open={showBehavior} onOpenChange={setShowBehavior}>
						<div className="border-0 border-t border-solid border-border pt-4">
							<CollapsibleTrigger asChild>
								<button
									type="button"
									className="flex w-full cursor-pointer items-start justify-between border-0 bg-transparent p-0 text-left transition-colors hover:text-content-primary"
								>
									<div>
										<h3 className="m-0 text-sm font-medium text-content-primary">
											Behavior
										</h3>
										<p className="m-0 text-xs text-content-secondary">
											Availability, model intent, and tool governance.
										</p>
									</div>
									{showBehavior ? (
										<ChevronDownIcon className="mt-0.5 h-4 w-4 shrink-0 text-content-secondary" />
									) : (
										<ChevronRightIcon className="mt-0.5 h-4 w-4 shrink-0 text-content-secondary" />
									)}
								</button>
							</CollapsibleTrigger>
							<CollapsibleContent className="space-y-4 pt-3">
								<Field label="Availability" htmlFor={`${formId}-availability`}>
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
															: {opt.description}
														</span>
													</div>
												</SelectItem>
											))}
										</SelectContent>
									</Select>
								</Field>

								<div className="flex items-start justify-between gap-4">
									<div className="min-w-0 space-y-1">
										<p className="m-0 text-sm font-medium text-content-primary">
											Model intent
										</p>
										<p className="m-0 text-xs text-content-secondary">
											Require the model to describe each tool call's purpose in
											natural language, shown as a status label in the UI.
										</p>
									</div>
									<Switch
										checked={form.values.modelIntent}
										aria-label="Model intent"
										onCheckedChange={(v) => {
											form.setFieldValue("modelIntent", v);
										}}
										disabled={isDisabled}
									/>
								</div>

								<div className="flex items-start justify-between gap-4">
									<div className="min-w-0 space-y-1">
										<Label
											htmlFor={`${formId}-allow-in-plan-mode`}
											className="text-sm font-medium text-content-primary"
										>
											Allow all tools from this MCP server in root plan mode
										</Label>
										<p className="m-0 text-xs text-content-secondary">
											When enabled, the root plan-mode agent can call these
											tools during planning. Workspace MCP and plan-mode
											subagents remain restricted.
										</p>
									</div>
									<Switch
										id={`${formId}-allow-in-plan-mode`}
										checked={form.values.allowInPlanMode}
										onCheckedChange={(v) => {
											form.setFieldValue("allowInPlanMode", v);
										}}
										disabled={isDisabled}
									/>
								</div>

								<div className="grid items-start gap-4 sm:grid-cols-2">
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
							</CollapsibleContent>
						</div>
					</Collapsible>
				</div>

				<div className="mt-auto py-6">
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
							<Button
								variant="outline"
								size="lg"
								type="button"
								onClick={onBack}
							>
								Cancel
							</Button>
						)}
						<Button size="lg" type="submit" disabled={!canSubmit}>
							{isSaving && <Spinner className="h-4 w-4" loading />}
							{isEditing ? "Save changes" : "Create server"}
						</Button>
					</div>
				</div>
			</form>
			{server && (
				<ConfirmDeleteDialog
					entity="MCP server"
					onConfirm={() => void onDelete(server.id)}
					isPending={isDeleting}
					open={confirmingDelete}
					onOpenChange={(open) => !open && setConfirmingDelete(false)}
				/>
			)}
		</div>
	);
};

// ── Main Panel ─────────────────────────────────────────────────

interface MCPServerAdminPanelProps {
	className?: string;
	sectionLabel?: string;
	sectionDescription?: string;
	sectionBadge?: ReactNode;
	// Data from query.
	serversData: TypesGen.MCPServerConfig[] | undefined;
	isLoadingServers: boolean;
	serversError: Error | null;
	// Mutation handlers.
	onCreateServer: (
		req: TypesGen.CreateMCPServerConfigRequest,
	) => Promise<unknown>;
	onUpdateServer: (args: {
		id: string;
		req: TypesGen.UpdateMCPServerConfigRequest;
	}) => Promise<unknown>;
	onDeleteServer: (id: string) => Promise<unknown>;
	isCreatingServer: boolean;
	isUpdatingServer: boolean;
	isDeletingServer: boolean;
	createError: Error | null;
	updateError: Error | null;
	deleteError: Error | null;
}

export const MCPServerAdminPanel: FC<MCPServerAdminPanelProps> = ({
	className,
	sectionLabel,
	sectionDescription,
	sectionBadge,
	serversData,
	isLoadingServers,
	serversError,
	onCreateServer,
	onUpdateServer,
	onDeleteServer,
	isCreatingServer,
	isUpdatingServer,
	isDeletingServer,
	createError,
	updateError,
	deleteError,
}) => {
	const [searchParams, setSearchParams] = useSearchParams();
	const serverId = searchParams.get("server");
	const navigate = useNavigate();
	const location = useLocation();
	// Whether the current form entry was pushed by an in-app click
	// (as opposed to a direct-entry URL like a bookmark or shared link).
	// When true, navigate(-1) is safe; otherwise we fall back to
	// clearing params with replace to avoid leaving the app.
	const canGoBack =
		(location.state as { pushed?: boolean } | null)?.pushed === true;

	const exitServerView = () => {
		if (canGoBack) {
			navigate(-1);
		} else {
			setSearchParams({}, { replace: true });
		}
	};

	const servers = (serversData ?? [])
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
				await onUpdateServer({ id, req: updateReq });
			} catch {
				// Error surfaced via mutation error state.
				return;
			}
		} else {
			try {
				await onCreateServer(req);
			} catch {
				// Error surfaced via mutation error state.
				return;
			}
		}
		exitServerView();
	};

	const handleDelete = async (id: string) => {
		try {
			await onDeleteServer(id);
		} catch {
			// Error surfaced via mutation error state.
			return;
		}
		exitServerView();
	};

	return (
		<div className={cn("flex min-h-full flex-col", className)}>
			{isLoadingServers && (
				<div className="flex items-center gap-1.5 text-xs text-content-secondary">
					<Spinner className="h-4 w-4" loading />
					Loading
				</div>
			)}
			{/* Content */}
			<div className="flex flex-1 flex-col gap-8">
				{!isFormView ? (
					<ServerList
						servers={servers}
						onSelect={(server) =>
							setSearchParams(
								{ server: server.id },
								{ state: { pushed: true } },
							)
						}
						onAdd={() =>
							setSearchParams({ server: "new" }, { state: { pushed: true } })
						}
						sectionLabel={sectionLabel}
						sectionDescription={sectionDescription}
						sectionBadge={sectionBadge}
					/>
				) : isCreating || (!isLoadingServers && editingServer) ? (
					<ServerForm
						key={serverId}
						server={isCreating ? null : editingServer}
						isSaving={isCreatingServer || isUpdatingServer}
						isDeleting={isDeletingServer}
						onSave={handleSave}
						onDelete={handleDelete}
						onBack={exitServerView}
					/>
				) : null}
			</div>

			{serversError && <ErrorAlert error={serversError} />}
			{createError && <ErrorAlert error={createError} />}
			{updateError && <ErrorAlert error={updateError} />}
			{deleteError && <ErrorAlert error={deleteError} />}
		</div>
	);
};
