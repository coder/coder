import { isAxiosError } from "axios";
import { KeyRoundIcon } from "lucide-react";
import { useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { useSearchParams } from "react-router";
import { type ApiErrorResponse, getErrorMessage } from "#/api/errors";
import {
	chatProjectExternalAuth,
	chatProjectExternalAuthDeviceCode,
	chatProjectExternalAuthKey,
	chatProjectMCPServerTokens,
	connectChatProjectExternalAuth,
	deleteChatProjectExternalAuth,
	deleteChatProjectMCPServerToken,
	exchangeChatProjectExternalAuthDevice,
	updateChatProjectMCPServerToken,
} from "#/api/queries/chatProjects";
import { mcpServerConfigs } from "#/api/queries/chats";
import type {
	ChatProject,
	ChatProjectMCPServerToken,
	ExternalAuthDevice,
	ExternalAuthLink,
	ExternalAuthLinkProvider,
	ListUserExternalAuthResponse,
} from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import { ConfirmDialog } from "#/components/Dialog/ConfirmDialog/ConfirmDialog";
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from "#/components/Dialog/Dialog";
import { ExternalImage } from "#/components/ExternalImage/ExternalImage";
import {
	GitDeviceAuth,
	isExchangeErrorRetryable,
	newRetryDelay,
} from "#/components/GitDeviceAuth/GitDeviceAuth";
import { Spinner } from "#/components/Spinner/Spinner";
import { toast } from "#/components/Toaster/toast";
import { MCPTokenDialog } from "#/modules/mcp/MCPTokenDialog";
import {
	type BuiltInMCPServerId,
	builtInTokenHelp,
	detectBuiltInMCPServer,
} from "#/modules/mcp/mcpPresets";
import { buildAgentProjectPath } from "../../utils/navigation";
import {
	SectionError,
	SectionLoadState,
	SettingsCard,
	SettingsCardEmptyRow,
	SettingsCardRow,
	SettingsCardRowIcon,
	SettingsCardRowText,
	SettingsSection,
} from "./ProjectSection";

const connectedSearchParam = "connected";

type ConnectedAccountsSectionProps = {
	readonly project: ChatProject;
};

/** The external accounts and MCP server credentials of the agent's user. */
export const ConnectedAccountsSection: React.FC<
	ConnectedAccountsSectionProps
> = ({ project }) => {
	const queryClient = useQueryClient();
	const authsQuery = useQuery(chatProjectExternalAuth(project));
	const connectMutation = useMutation(connectChatProjectExternalAuth(project));
	const disconnectMutation = useMutation(
		deleteChatProjectExternalAuth(queryClient, project),
	);
	const tokensQuery = useQuery(chatProjectMCPServerTokens(project));
	const mcpConfigsQuery = useQuery(mcpServerConfigs(project.organization_id));
	// Which built-in server each token row is, for its help link and for
	// offering a GitHub App.
	const builtInServers = new Map(
		(tokensQuery.data ?? []).flatMap((server) => {
			const url = mcpConfigsQuery.data?.find(
				(config) => config.id === server.mcp_server_config_id,
			)?.url;
			const builtIn = detectBuiltInMCPServer({ slug: server.slug, url });
			return builtIn ? [[server.mcp_server_config_id, builtIn] as const] : [];
		}),
	);
	const saveTokenMutation = useMutation(
		updateChatProjectMCPServerToken(queryClient, project),
	);
	const removeTokenMutation = useMutation(
		deleteChatProjectMCPServerToken(queryClient, project),
	);
	const [tokenTarget, setTokenTarget] = useState<ChatProjectMCPServerToken>();
	const [connectingId, setConnectingId] = useState<string>();
	const [searchParams, setSearchParams] = useSearchParams();
	const justConnected = searchParams.has(connectedSearchParam);

	// The browser may restore this page from its back/forward cache after the
	// provider redirect, with the connect button still pending.
	useEffect(() => {
		const handlePageShow = (event: PageTransitionEvent) => {
			if (event.persisted) {
				setConnectingId(undefined);
			}
		};
		window.addEventListener("pageshow", handlePageShow);
		return () => window.removeEventListener("pageshow", handlePageShow);
	}, []);

	// The provider flow returns here with the connected provider in the URL.
	useEffect(() => {
		if (!justConnected) {
			return;
		}
		toast.success("Account connected");
		setSearchParams(
			(params) => {
				const next = new URLSearchParams(params);
				next.delete(connectedSearchParam);
				return next;
			},
			{ replace: true },
		);
	}, [justConnected, setSearchParams]);
	const [deviceProvider, setDeviceProvider] =
		useState<ExternalAuthLinkProvider>();
	const [providerToDisconnect, setProviderToDisconnect] =
		useState<ExternalAuthLinkProvider>();

	const targetServer = tokenTarget
		? builtInServers.get(tokenTarget.mcp_server_config_id)
		: undefined;

	return (
		<>
			<ConnectedAccountsSectionView
				auths={authsQuery.data}
				error={authsQuery.error}
				onRetry={() => void authsQuery.refetch()}
				mcpTokens={(tokensQuery.data ?? []).filter((t) => t.token_required)}
				canManageAccounts={project.permissions.manage_triggers}
				builtInServers={builtInServers}
				onEditMcpToken={(server) => {
					saveTokenMutation.reset();
					removeTokenMutation.reset();
					setTokenTarget(server);
				}}
				connectingProviderId={connectingId}
				onConnect={(provider) => {
					if (provider.device) {
						setDeviceProvider(provider);
						return;
					}
					setConnectingId(provider.id);
					connectMutation.mutate(
						{
							providerId: provider.id,
							redirect: `${buildAgentProjectPath({
								projectId: project.id,
								tab: "settings",
							})}?${connectedSearchParam}=${encodeURIComponent(provider.id)}`,
						},
						{
							onSuccess: (response) => {
								location.href = response.url;
							},
							onError: (error) => {
								setConnectingId(undefined);
								toast.error(
									getErrorMessage(error, "Failed to connect the account."),
								);
							},
						},
					);
				}}
				onDisconnect={setProviderToDisconnect}
			/>
			{tokenTarget && (
				<MCPTokenDialog
					serverName={tokenTarget.display_name}
					audience="Chats in this agent"
					hasCredential={tokenTarget.has_token}
					help={targetServer && builtInTokenHelp[targetServer]}
					githubApp={
						targetServer === "github"
							? {
									current:
										tokenTarget.credential_kind === "github_app"
											? tokenTarget.github_app
											: undefined,
									onSave: (app) =>
										saveTokenMutation.mutate(
											{
												serverId: tokenTarget.mcp_server_config_id,
												request: {
													credential_kind: "github_app",
													github_app: app,
												},
											},
											{ onSuccess: () => setTokenTarget(undefined) },
										),
								}
							: undefined
					}
					isSaving={saveTokenMutation.isPending}
					isRemoving={removeTokenMutation.isPending}
					error={saveTokenMutation.error ?? removeTokenMutation.error}
					onSave={(token) =>
						saveTokenMutation.mutate(
							{
								serverId: tokenTarget.mcp_server_config_id,
								request: { credential_kind: "token", token },
							},
							{ onSuccess: () => setTokenTarget(undefined) },
						)
					}
					onRemove={() =>
						removeTokenMutation.mutate(tokenTarget.mcp_server_config_id, {
							onSuccess: () => setTokenTarget(undefined),
						})
					}
					onClose={() => setTokenTarget(undefined)}
				/>
			)}
			{deviceProvider && (
				<ConnectDeviceDialog
					key={deviceProvider.id}
					project={project}
					provider={deviceProvider}
					onClose={() => setDeviceProvider(undefined)}
				/>
			)}
			<ConfirmDialog
				type="delete"
				open={providerToDisconnect !== undefined}
				title="Disconnect account?"
				description={
					<>
						Chats in this agent will no longer be able to use the{" "}
						<strong>
							{providerToDisconnect?.display_name || providerToDisconnect?.id}
						</strong>{" "}
						account.
					</>
				}
				confirmText="Disconnect"
				confirmLoading={disconnectMutation.isPending}
				onClose={() => setProviderToDisconnect(undefined)}
				onConfirm={() => {
					if (!providerToDisconnect) {
						return;
					}
					disconnectMutation.mutate(providerToDisconnect.id, {
						onSuccess: () => {
							toast.success("Account disconnected");
							setProviderToDisconnect(undefined);
						},
						onError: (error) =>
							toast.error(
								getErrorMessage(error, "Failed to disconnect the account."),
							),
					});
				}}
			/>
		</>
	);
};

type ConnectedAccountsSectionViewProps = {
	/** The agent's MCP servers that need a token of the agent's user. */
	readonly mcpTokens: readonly ChatProjectMCPServerToken[];
	/** The built-in server each `mcpTokens` row is, by MCP server ID. */
	readonly builtInServers: ReadonlyMap<string, BuiltInMCPServerId>;
	readonly onEditMcpToken: (server: ChatProjectMCPServerToken) => void;
	/** `undefined` until the accounts load. */
	readonly auths: ListUserExternalAuthResponse | undefined;
	readonly error: unknown;
	readonly onRetry: () => void;
	/** Shows the connect, disconnect, and credential actions. */
	readonly canManageAccounts: boolean;
	/** The provider whose connect request is in flight or redirecting. */
	readonly connectingProviderId: string | undefined;
	readonly onConnect: (provider: ExternalAuthLinkProvider) => void;
	readonly onDisconnect: (provider: ExternalAuthLinkProvider) => void;
};

/** One row per external auth provider and per MCP server that needs a token. */
export const ConnectedAccountsSectionView: React.FC<
	ConnectedAccountsSectionViewProps
> = ({
	auths,
	error,
	onRetry,
	canManageAccounts,
	mcpTokens,
	builtInServers,
	onEditMcpToken,
	connectingProviderId,
	onConnect,
	onDisconnect,
}) => (
	<SettingsSection
		id="connected-accounts"
		title="Connected accounts"
		description="Accounts and tokens that this agent's user uses, such as GitHub."
	>
		{auths === undefined ? (
			<SectionLoadState error={error} onRetry={onRetry} />
		) : (
			<>
				{Boolean(error) && <SectionError error={error} onRetry={onRetry} />}
				<SettingsCard>
					{auths.providers.length === 0 && mcpTokens.length === 0 && (
						<SettingsCardEmptyRow>
							No external accounts or MCP server credentials are set up.
						</SettingsCardEmptyRow>
					)}
					{auths.providers.map((provider) => (
						<AccountRow
							key={provider.id}
							provider={provider}
							link={auths.links.find((l) => l.provider_id === provider.id)}
							canEdit={canManageAccounts}
							isConnecting={connectingProviderId === provider.id}
							onConnect={() => onConnect(provider)}
							onDisconnect={() => onDisconnect(provider)}
						/>
					))}
					{mcpTokens.map((server) => (
						<MCPTokenRow
							key={server.mcp_server_config_id}
							server={server}
							canManage={canManageAccounts}
							isGitHub={
								builtInServers.get(server.mcp_server_config_id) === "github"
							}
							onEdit={() => onEditMcpToken(server)}
						/>
					))}
				</SettingsCard>
			</>
		)}
	</SettingsSection>
);

type AccountRowProps = {
	readonly provider: ExternalAuthLinkProvider;
	readonly link: ExternalAuthLink | undefined;
	readonly canEdit: boolean;
	readonly isConnecting: boolean;
	readonly onConnect: () => void;
	readonly onDisconnect: () => void;
};

type MCPTokenRowProps = {
	readonly server: ChatProjectMCPServerToken;
	readonly canManage: boolean;
	readonly isGitHub: boolean;
	readonly onEdit: () => void;
};

type MCPTokenStatusProps = {
	readonly server: ChatProjectMCPServerToken;
};

/** Says whether the agent's user has a credential for the MCP server. */
const MCPTokenStatus: React.FC<MCPTokenStatusProps> = ({ server }) => {
	if (!server.has_token) {
		return <span className="text-content-warning">Token needed</span>;
	}
	if (server.credential_kind !== "github_app") {
		return <span className="text-content-success">Connected</span>;
	}
	// The App's details are only sent to the agent's owner.
	const app = server.github_app;
	if (!app) {
		return <span className="text-content-success">GitHub App</span>;
	}
	return (
		<>
			<span className="text-content-success">
				GitHub App {app.app_id}, installation {app.installation_id}
			</span>
			{app.repositories.length > 0 &&
				`, repositories: ${app.repositories.join(", ")}`}
			{app.use_for_git && ", used for git"}
		</>
	);
};

/** An MCP server that needs a credential, shown like an external account. */
const MCPTokenRow: React.FC<MCPTokenRowProps> = ({
	server,
	canManage,
	isGitHub,
	onEdit,
}) => {
	// A GitHub server can use a GitHub App, so its credential is not only a token.
	const noun = isGitHub ? "credential" : "token";
	return (
		<SettingsCardRow>
			<SettingsCardRowIcon>
				{server.icon_url ? (
					<ExternalImage src={server.icon_url} alt="" />
				) : (
					<KeyRoundIcon aria-hidden className="text-content-secondary" />
				)}
			</SettingsCardRowIcon>
			<SettingsCardRowText
				title={server.display_name}
				description={<MCPTokenStatus server={server} />}
			/>
			{canManage && (
				<Button
					size="sm"
					variant="outline"
					aria-label={`${server.has_token ? "Replace" : "Add"} ${noun} for ${server.display_name}`}
					onClick={onEdit}
				>
					{server.has_token ? "Replace" : "Add"} {noun}
				</Button>
			)}
		</SettingsCardRow>
	);
};

type AccountStatus = {
	readonly label: string;
	readonly className?: string;
};

const accountStatus = (link: ExternalAuthLink | undefined): AccountStatus => {
	if (!link) {
		return { label: "Not connected" };
	}
	if (link.authenticated) {
		return { label: "Connected", className: "text-content-success" };
	}
	return {
		label: link.validate_error
			? `Needs reconnecting: ${link.validate_error}`
			: "Needs reconnecting",
		className: "text-content-warning",
	};
};

const AccountRow: React.FC<AccountRowProps> = ({
	provider,
	link,
	canEdit,
	isConnecting,
	onConnect,
	onDisconnect,
}) => {
	const name = provider.display_name || provider.id;
	const status = accountStatus(link);

	return (
		<SettingsCardRow>
			<SettingsCardRowIcon>
				{provider.display_icon ? (
					<ExternalImage src={provider.display_icon} alt="" />
				) : (
					<KeyRoundIcon aria-hidden className="text-content-secondary" />
				)}
			</SettingsCardRowIcon>
			<SettingsCardRowText
				title={name}
				description={<span className={status.className}>{status.label}</span>}
			/>
			{canEdit && (
				<div className="flex shrink-0 items-center gap-2">
					{link && (
						<Button
							size="sm"
							variant="outline"
							aria-label={`Disconnect ${name}`}
							onClick={onDisconnect}
						>
							Disconnect
						</Button>
					)}
					{!link?.authenticated && (
						<Button
							size="sm"
							variant="outline"
							aria-label={`${link ? "Reconnect" : "Connect"} ${name}`}
							disabled={isConnecting}
							onClick={onConnect}
						>
							<Spinner loading={isConnecting} />
							{link ? "Reconnect" : "Connect"}
						</Button>
					)}
				</div>
			)}
		</SettingsCardRow>
	);
};

type ConnectDeviceDialogProps = {
	readonly project: ChatProject;
	readonly provider: ExternalAuthLinkProvider;
	readonly onClose: () => void;
};

/** Shows the one-time code for a device-flow provider and waits for approval. */
const ConnectDeviceDialog: React.FC<ConnectDeviceDialogProps> = ({
	project,
	provider,
	onClose,
}) => {
	const queryClient = useQueryClient();
	const deviceQuery = useQuery({
		...chatProjectExternalAuthDeviceCode(project, provider.id),
		refetchOnMount: "always",
	});
	// Never show or poll a code left over from an earlier visit.
	const device = deviceQuery.isFetchedAfterMount ? deviceQuery.data : undefined;
	const retryDelay = newRetryDelay(device?.interval);
	const exchange = exchangeChatProjectExternalAuthDevice(
		project,
		provider.id,
		device?.device_code ?? "",
	);
	const exchangeQuery = useQuery({
		...exchange,
		// Finish the connection when the exchange succeeds, so no effect has to
		// watch the query status.
		queryFn: async (context) => {
			if (typeof exchange.queryFn !== "function") {
				throw new Error("The device code exchange has no query function.");
			}
			const result = await exchange.queryFn(context);
			// Closing the dialog aborts the query. A late result must not
			// announce a connection the user no longer waits for.
			if (context.signal.aborted) {
				return result;
			}
			void queryClient.invalidateQueries({
				queryKey: chatProjectExternalAuthKey(project.id),
				exact: true,
			});
			toast.success("Account connected");
			onClose();
			return result;
		},
		enabled: Boolean(device),
		retry: isExchangeErrorRetryable,
		retryDelay,
		// The provider rate limits polling, so only the retry policy may refetch.
		refetchOnWindowFocus: false,
	});

	return (
		<ConnectDeviceDialogView
			providerName={provider.display_name || provider.id}
			device={device}
			deviceExchangeError={
				apiErrorOf(exchangeQuery.failureReason) ??
				apiErrorOf(deviceQuery.failureReason)
			}
			onClose={onClose}
		/>
	);
};

const apiErrorOf = (reason: unknown): ApiErrorResponse | undefined =>
	isAxiosError<ApiErrorResponse>(reason) ? reason.response?.data : undefined;

type ConnectDeviceDialogViewProps = {
	readonly providerName: string;
	readonly device: ExternalAuthDevice | undefined;
	readonly deviceExchangeError: ApiErrorResponse | undefined;
	readonly onClose: () => void;
};

/** The device-flow dialog: the provider's one-time code and a Cancel button. */
export const ConnectDeviceDialogView: React.FC<
	ConnectDeviceDialogViewProps
> = ({ providerName, device, deviceExchangeError, onClose }) => (
	<Dialog
		open
		onOpenChange={(open) => {
			if (!open) {
				onClose();
			}
		}}
	>
		<DialogContent>
			<DialogHeader>
				<DialogTitle>Connect {providerName}</DialogTitle>
				<DialogDescription>
					Authorize the agent's user on {providerName}. Chats in this agent use
					this account, so prefer a dedicated bot account over a personal one.
					This dialog closes when the provider confirms.
				</DialogDescription>
			</DialogHeader>
			<GitDeviceAuth
				externalAuthDevice={device}
				deviceExchangeError={deviceExchangeError}
			/>
			<DialogFooter>
				<Button type="button" variant="outline" onClick={onClose}>
					Cancel
				</Button>
			</DialogFooter>
		</DialogContent>
	</Dialog>
);
