import { ServerIcon } from "lucide-react";
import { type FC, useId, useMemo, useState } from "react";
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
import { Input } from "#/components/Input/Input";
import { Loader } from "#/components/Loader/Loader";
import { Spinner } from "#/components/Spinner/Spinner";
import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from "#/components/Table/Table";
import { TableEmpty } from "#/components/TableEmpty/TableEmpty";
import { cn } from "#/utils/cn";

// ── Helpers ────────────────────────────────────────────────────

/**
 * A server is shown to the user when it is enabled AND requires
 * per-user action: either OAuth2 sign-in, or admin-marked
 * custom-header keys for the user to supply.
 */
export const filterUserConfigurableServers = (
	servers: readonly TypesGen.MCPServerConfig[],
): TypesGen.MCPServerConfig[] =>
	servers.filter(
		(s) =>
			s.enabled &&
			(s.auth_type === "oauth2" ||
				(s.auth_type === "custom_headers" &&
					(s.custom_headers_user_keys?.length ?? 0) > 0)),
	);

// ── Server icon ────────────────────────────────────────────────

const MCPServerIcon: FC<{ iconUrl: string; name: string }> = ({
	iconUrl,
	name,
}) => {
	if (iconUrl) {
		return (
			<ExternalImage
				src={iconUrl}
				alt={name}
				className="size-6 rounded-sm object-cover"
			/>
		);
	}
	return (
		<ServerIcon aria-hidden="true" className="size-6 text-content-secondary" />
	);
};

// ── Props ──────────────────────────────────────────────────────

export interface AgentSettingsUserMCPServersPageViewProps {
	readonly servers: readonly TypesGen.MCPServerConfig[] | undefined;
	readonly isLoadingServers: boolean;
	readonly serversError: unknown;
	/** Map of server id → `has_values` reported by the API. */
	readonly headerValueStatus: Readonly<Record<string, Record<string, boolean>>>;
	/** Server ids whose header status is still loading. */
	readonly loadingHeaderStatusIds: ReadonlySet<string>;
	readonly onConnectOAuth2: (server: TypesGen.MCPServerConfig) => void;
	readonly onSaveHeaderValues: (
		server: TypesGen.MCPServerConfig,
		values: Record<string, string>,
	) => Promise<unknown>;
	readonly onClearHeaderValues: (
		server: TypesGen.MCPServerConfig,
	) => Promise<unknown>;
	readonly isSavingHeaderValues: boolean;
	readonly isClearingHeaderValues: boolean;
	readonly saveHeaderValuesError: unknown;
}

// ── Page view ──────────────────────────────────────────────────

export const AgentSettingsUserMCPServersPageView: FC<
	AgentSettingsUserMCPServersPageViewProps
> = ({
	servers,
	isLoadingServers,
	serversError,
	headerValueStatus,
	loadingHeaderStatusIds,
	onConnectOAuth2,
	onSaveHeaderValues,
	onClearHeaderValues,
	isSavingHeaderValues,
	isClearingHeaderValues,
	saveHeaderValuesError,
}) => {
	const [configuringServer, setConfiguringServer] =
		useState<TypesGen.MCPServerConfig | null>(null);

	const visibleServers = useMemo(
		() => filterUserConfigurableServers(servers ?? []),
		[servers],
	);

	if (serversError) {
		return <ErrorAlert error={serversError} />;
	}

	if (isLoadingServers || !servers) {
		return <Loader />;
	}

	return (
		<div className="space-y-4">
			<header className="space-y-1">
				<h2 className="m-0 text-base font-semibold text-content-primary">
					MCP Servers
				</h2>
				<p className="m-0 text-sm text-content-secondary">
					Connect or supply per-user credentials for MCP servers that require
					action from you. Admin-set values are managed by your workspace
					administrator.
				</p>
			</header>

			<Table>
				<TableHeader>
					<TableRow>
						<TableHead>Server</TableHead>
						<TableHead>Status</TableHead>
						<TableHead className="w-[1%] text-right">
							<span className="sr-only">Actions</span>
						</TableHead>
					</TableRow>
				</TableHeader>
				<TableBody>
					{visibleServers.length === 0 ? (
						<TableEmpty message="No MCP servers require your action right now." />
					) : (
						visibleServers.map((server) => (
							<ServerRow
								key={server.id}
								server={server}
								statusLoading={loadingHeaderStatusIds.has(server.id)}
								headerHasValues={headerValueStatus[server.id]}
								onConnectOAuth2={() => onConnectOAuth2(server)}
								onConfigure={() => setConfiguringServer(server)}
							/>
						))
					)}
				</TableBody>
			</Table>

			<ConfigureHeadersDialog
				server={configuringServer}
				headerHasValues={
					configuringServer
						? headerValueStatus[configuringServer.id]
						: undefined
				}
				onClose={() => setConfiguringServer(null)}
				onSave={onSaveHeaderValues}
				onClear={onClearHeaderValues}
				isSaving={isSavingHeaderValues}
				isClearing={isClearingHeaderValues}
				error={saveHeaderValuesError}
			/>
		</div>
	);
};

// ── Server row ─────────────────────────────────────────────────

interface ServerRowProps {
	readonly server: TypesGen.MCPServerConfig;
	readonly statusLoading: boolean;
	readonly headerHasValues: Record<string, boolean> | undefined;
	readonly onConnectOAuth2: () => void;
	readonly onConfigure: () => void;
}

const ServerRow: FC<ServerRowProps> = ({
	server,
	statusLoading,
	headerHasValues,
	onConnectOAuth2,
	onConfigure,
}) => {
	const isOAuth2 = server.auth_type === "oauth2";
	const isCustomHeaders = server.auth_type === "custom_headers";

	const requiredKeys = server.custom_headers_user_keys ?? [];
	const hasAllValues =
		isCustomHeaders &&
		requiredKeys.length > 0 &&
		requiredKeys.every((k) => headerHasValues?.[k] === true);

	const connected = isOAuth2 ? server.auth_connected : hasAllValues;
	const statusText = connected ? "Connected" : "Action required";
	const statusClass = connected
		? "text-content-success"
		: "text-content-warning";

	return (
		<TableRow>
			<TableCell>
				<div className="flex items-center gap-2.5">
					<MCPServerIcon iconUrl={server.icon_url} name={server.display_name} />
					<div className="flex flex-col">
						<span className="font-semibold text-content-primary">
							{server.display_name}
						</span>
						{server.description && (
							<span className="text-xs text-content-secondary">
								{server.description}
							</span>
						)}
					</div>
				</div>
			</TableCell>
			<TableCell>
				{statusLoading ? (
					<Spinner loading />
				) : (
					<span className={cn("text-sm font-medium", statusClass)}>
						{statusText}
					</span>
				)}
			</TableCell>
			<TableCell className="text-right">
				{isOAuth2 && !server.auth_connected && (
					<Button size="sm" onClick={onConnectOAuth2}>
						Connect
					</Button>
				)}
				{isOAuth2 && server.auth_connected && (
					<span className="text-sm text-content-secondary">Signed in</span>
				)}
				{isCustomHeaders && (
					<Button size="sm" variant="outline" onClick={onConfigure}>
						{connected ? "Edit" : "Configure"}
					</Button>
				)}
			</TableCell>
		</TableRow>
	);
};

// ── Configure dialog ───────────────────────────────────────────

interface ConfigureHeadersDialogProps {
	readonly server: TypesGen.MCPServerConfig | null;
	readonly headerHasValues: Record<string, boolean> | undefined;
	readonly onClose: () => void;
	readonly onSave: (
		server: TypesGen.MCPServerConfig,
		values: Record<string, string>,
	) => Promise<unknown>;
	readonly onClear: (server: TypesGen.MCPServerConfig) => Promise<unknown>;
	readonly isSaving: boolean;
	readonly isClearing: boolean;
	readonly error: unknown;
}

const ConfigureHeadersDialog: FC<ConfigureHeadersDialogProps> = ({
	server,
	headerHasValues,
	onClose,
	onSave,
	onClear,
	isSaving,
	isClearing,
	error,
}) => {
	const inputIdPrefix = useId();
	const requiredKeys = useMemo(
		() => server?.custom_headers_user_keys ?? [],
		[server],
	);
	const descriptions = useMemo(
		() => server?.custom_headers_user_key_descriptions ?? {},
		[server],
	);
	const [draft, setDraft] = useState<Record<string, string>>({});

	const open = server !== null;
	const hasAnyExisting = requiredKeys.some(
		(k) => headerHasValues?.[k] === true,
	);
	const isBusy = isSaving || isClearing;

	const handleSave = async () => {
		if (!server) return;
		const values: Record<string, string> = {};
		for (const key of requiredKeys) {
			const v = draft[key];
			if (typeof v === "string" && v !== "") {
				values[key] = v;
			}
		}
		await onSave(server, values);
		setDraft({});
		onClose();
	};

	const handleClear = async () => {
		if (!server) return;
		await onClear(server);
		setDraft({});
		onClose();
	};

	return (
		<Dialog
			open={open}
			onOpenChange={(nextOpen) => {
				if (!nextOpen) {
					setDraft({});
					onClose();
				}
			}}
		>
			<DialogContent>
				<DialogHeader>
					<DialogTitle>
						Configure {server?.display_name ?? "MCP server"}
					</DialogTitle>
					<DialogDescription>
						Provide values for the per-user headers required by this server.
						Leave a field blank to keep the existing value.
					</DialogDescription>
				</DialogHeader>

				{error ? <ErrorAlert error={error} /> : null}

				<form
					id="mcp-user-headers-form"
					className="space-y-3"
					onSubmit={(e) => {
						e.preventDefault();
						void handleSave();
					}}
				>
					{requiredKeys.map((key) => {
						const existing = headerHasValues?.[key] === true;
						const inputId = `${inputIdPrefix}-${key}`;
						const description = descriptions[key];
						return (
							<div key={key} className="space-y-1">
								<label
									htmlFor={inputId}
									className="text-xs font-medium text-content-primary"
								>
									{key}
									{existing && (
										<span className="ml-2 text-[10px] uppercase tracking-wide text-content-secondary">
											value set
										</span>
									)}
								</label>
								{description && (
									<p className="m-0 text-xs text-content-secondary">
										{description}
									</p>
								)}
								<Input
									id={inputId}
									className="font-mono text-[13px] [-webkit-text-security:disc]"
									type="text"
									autoComplete="off"
									data-1p-ignore
									data-lpignore="true"
									data-form-type="other"
									data-bwignore
									value={draft[key] ?? ""}
									onChange={(e) =>
										setDraft((d) => ({ ...d, [key]: e.target.value }))
									}
									placeholder={existing ? "Replace value" : "Enter value"}
									aria-label={`${key} value`}
									disabled={isBusy}
								/>
							</div>
						);
					})}
				</form>

				<DialogFooter>
					{hasAnyExisting && (
						<Button
							variant="outline"
							type="button"
							onClick={() => void handleClear()}
							disabled={isBusy}
						>
							<Spinner loading={isClearing} />
							Clear all
						</Button>
					)}
					<Button type="submit" form="mcp-user-headers-form" disabled={isBusy}>
						<Spinner loading={isSaving} />
						Save
					</Button>
				</DialogFooter>
			</DialogContent>
		</Dialog>
	);
};
