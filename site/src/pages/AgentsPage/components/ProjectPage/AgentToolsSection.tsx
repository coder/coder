import { WrenchIcon, XIcon } from "lucide-react";
import { useRef } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import {
	chatProjectMCPServerTokens,
	updateChatProject,
} from "#/api/queries/chatProjects";
import { mcpServerConfigs } from "#/api/queries/chats";
import type { ChatProject, MCPServerConfig } from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import {
	Combobox,
	ComboboxContent,
	ComboboxEmpty,
	ComboboxInput,
	ComboboxItem,
	ComboboxList,
	ComboboxTrigger,
} from "#/components/Combobox/Combobox";
import { ExternalImage } from "#/components/ExternalImage/ExternalImage";
import { Link } from "#/components/Link/Link";
import {
	SectionError,
	SectionLoadState,
	SectionSaveError,
	SettingsCard,
	SettingsCardAddRow,
	SettingsCardEmptyRow,
	SettingsCardRow,
	SettingsCardRowIcon,
	SettingsCardRowText,
	SettingsSection,
} from "./ProjectSection";
import { useSingleFlight } from "./useSingleFlight";

type MCPServerOption = {
	readonly id: string;
	readonly label: string;
	readonly iconURL: string;
	/** True when the agent's user has no token for this server yet. */
	readonly needsToken?: boolean;
};

const toOption = (server: MCPServerConfig): MCPServerOption => ({
	id: server.id,
	label: `${server.display_name || server.slug}${server.enabled ? "" : " (disabled)"}`,
	iconURL: server.icon_url,
});

type AgentToolsSectionProps = {
	readonly project: ChatProject;
};

/**
 * Lists the MCP servers attached to every chat in an agent and adds or
 * removes them as soon as they are picked.
 */
export const AgentToolsSection: React.FC<AgentToolsSectionProps> = ({
	project,
}) => {
	const queryClient = useQueryClient();
	const mcpQuery = useQuery(mcpServerConfigs(project.organization_id));
	const updateMutation = useMutation(updateChatProject(queryClient));
	const singleFlight = useSingleFlight();

	const tokensQuery = useQuery(chatProjectMCPServerTokens(project));

	const servers = mcpQuery.data ?? [];
	// A disabled server stays listed while it is attached, so it can be removed.
	const attached: MCPServerOption[] = project.mcp_server_ids.map((id) => {
		const server = servers.find((s) => s.id === id);
		const token = tokensQuery.data?.find((t) => t.mcp_server_config_id === id);
		return server
			? {
					...toOption(server),
					needsToken: token?.token_required && !token.has_token,
				}
			: { id, label: "Unknown MCP server", iconURL: "" };
	});
	const available = servers
		.filter((s) => s.enabled && !project.mcp_server_ids.includes(s.id))
		.map(toOption);

	const save = (mcpServerIds: string[]) => {
		singleFlight.run((onSettled) =>
			updateMutation.mutate(
				{ project, request: { mcp_server_ids: mcpServerIds } },
				{ onSettled },
			),
		);
	};

	return (
		<AgentToolsSectionView
			attached={attached}
			available={available}
			isReady={mcpQuery.data !== undefined}
			error={mcpQuery.error}
			onRetry={() => void mcpQuery.refetch()}
			canEdit={project.permissions.edit}
			isSaving={updateMutation.isPending}
			saveError={updateMutation.error}
			onAdd={(id) => save([...project.mcp_server_ids, id])}
			onRemove={(id) => save(project.mcp_server_ids.filter((s) => s !== id))}
		/>
	);
};

type AgentToolsSectionViewProps = {
	/** The MCP servers attached to the agent. */
	readonly attached: readonly MCPServerOption[];
	/** Enabled MCP servers that can still be attached. */
	readonly available: readonly MCPServerOption[];
	/** False until the MCP servers have loaded. */
	readonly isReady: boolean;
	/** Shown with Retry, above the list when it is already loaded. */
	readonly error: unknown;
	readonly onRetry: () => void;
	readonly canEdit: boolean;
	/** True while a change saves. Controls stay focusable but ignore input. */
	readonly isSaving: boolean;
	readonly saveError: unknown;
	readonly onAdd: (id: string) => void;
	readonly onRemove: (id: string) => void;
};

/** The MCP servers of an agent as card rows, with a searchable add menu. */
export const AgentToolsSectionView: React.FC<AgentToolsSectionViewProps> = ({
	attached,
	available,
	isReady,
	error,
	onRetry,
	canEdit,
	isSaving,
	saveError,
	onAdd,
	onRemove,
}) => {
	const removeButtons = useRef(new Map<string, HTMLButtonElement>());
	const addButtonRef = useRef<HTMLButtonElement>(null);

	// The removed row disappears, so focus goes to the next tool's Remove
	// button, or to Add MCP server after the last row.
	const focusAfterRemoving = (id: string) => {
		const index = attached.findIndex((server) => server.id === id);
		const next = attached[index + 1];
		const target = next ? removeButtons.current.get(next.id) : undefined;
		(target ?? addButtonRef.current)?.focus();
	};

	return (
		<SettingsSection
			title="Tools"
			description="MCP servers attached to every chat in this agent."
		>
			{!isReady ? (
				<SectionLoadState error={error} onRetry={onRetry} />
			) : (
				<>
					{Boolean(error) && <SectionError error={error} onRetry={onRetry} />}
					{Boolean(saveError) && <SectionSaveError error={saveError} />}
					<SettingsCard>
						{attached.length === 0 && (
							<SettingsCardEmptyRow>No MCP servers yet.</SettingsCardEmptyRow>
						)}
						{attached.map((server) => (
							<ToolRow
								key={server.id}
								server={server}
								canEdit={canEdit}
								isSaving={isSaving}
								removeButtonRef={(element) => {
									if (element) {
										removeButtons.current.set(server.id, element);
									}
									return () => {
										removeButtons.current.delete(server.id);
									};
								}}
								onRemove={() => {
									focusAfterRemoving(server.id);
									onRemove(server.id);
								}}
							/>
						))}
						{canEdit && (
							<AddToolRow
								available={available}
								hasAttached={attached.length > 0}
								isSaving={isSaving}
								addButtonRef={addButtonRef}
								onAdd={onAdd}
							/>
						)}
					</SettingsCard>
				</>
			)}
		</SettingsSection>
	);
};

type ToolRowProps = {
	readonly server: MCPServerOption;
	readonly canEdit: boolean;
	readonly isSaving: boolean;
	readonly removeButtonRef: React.Ref<HTMLButtonElement>;
	readonly onRemove: () => void;
};

const ToolRow: React.FC<ToolRowProps> = ({
	server,
	canEdit,
	isSaving,
	removeButtonRef,
	onRemove,
}) => (
	<SettingsCardRow>
		<SettingsCardRowIcon>
			<ToolIcon server={server} />
		</SettingsCardRowIcon>
		<SettingsCardRowText
			title={server.label}
			description={
				server.needsToken ? (
					<Link href="#connected-accounts" size="sm">
						Token needed
					</Link>
				) : undefined
			}
		/>
		{canEdit && (
			<Button
				size="icon"
				variant="subtle"
				aria-disabled={isSaving}
				className="aria-disabled:cursor-not-allowed aria-disabled:opacity-50"
				aria-label={`Remove ${server.label}`}
				ref={removeButtonRef}
				onClick={() => {
					if (!isSaving) {
						onRemove();
					}
				}}
			>
				<XIcon />
			</Button>
		)}
	</SettingsCardRow>
);

type ToolIconProps = {
	readonly server: MCPServerOption;
};

const ToolIcon: React.FC<ToolIconProps> = ({ server }) =>
	server.iconURL ? (
		<ExternalImage src={server.iconURL} alt="" />
	) : (
		<WrenchIcon aria-hidden className="text-content-secondary" />
	);

type AddToolRowProps = Pick<
	AgentToolsSectionViewProps,
	"available" | "isSaving" | "onAdd"
> & {
	readonly hasAttached: boolean;
	readonly addButtonRef: React.Ref<HTMLButtonElement>;
};

const AddToolRow: React.FC<AddToolRowProps> = ({
	available,
	hasAttached,
	isSaving,
	addButtonRef,
	onAdd,
}) => {
	let emptyLabel = "No MCP servers are configured.";
	if (available.length > 0) {
		emptyLabel = "No MCP servers found.";
	} else if (hasAttached) {
		emptyLabel = "Every enabled MCP server is attached.";
	}
	return (
		<Combobox>
			<ComboboxTrigger asChild>
				<SettingsCardAddRow
					ref={addButtonRef}
					label="Add MCP server"
					aria-disabled={isSaving}
					onClick={(event) => {
						// Keeps the menu closed while a change saves.
						if (isSaving) {
							event.preventDefault();
						}
					}}
				/>
			</ComboboxTrigger>
			<ComboboxContent align="start" className="w-80">
				<ComboboxInput placeholder="Search MCP servers" />
				<ComboboxList>
					{available.map((server) => (
						<ComboboxItem
							key={server.id}
							value={server.id}
							keywords={[server.label]}
							onSelect={() => onAdd(server.id)}
						>
							<SettingsCardRowIcon>
								<ToolIcon server={server} />
							</SettingsCardRowIcon>
							{server.label}
						</ComboboxItem>
					))}
				</ComboboxList>
				<ComboboxEmpty>{emptyLabel}</ComboboxEmpty>
			</ComboboxContent>
		</Combobox>
	);
};
