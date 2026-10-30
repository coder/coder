import { XIcon } from "lucide-react";
import { useRef } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { chatProjects, updateChatProject } from "#/api/queries/chatProjects";
import type {
	ChatProject,
	UpdateChatProjectRequest,
} from "#/api/typesGenerated";
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
import { ChatProjectIcon } from "../ChatProjectIcon";
import {
	SectionError,
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

type InvokableAgentsSectionProps = {
	readonly project: ChatProject;
};

/**
 * The other agents that chats in an agent may hand tasks to. Each change
 * saves as soon as it is made.
 */
export const InvokableAgentsSection: React.FC<InvokableAgentsSectionProps> = ({
	project,
}) => {
	const queryClient = useQueryClient();
	// The page already loaded this list to resolve the agent; refetching on
	// mount would duplicate the request.
	const agentsQuery = useQuery({ ...chatProjects(), refetchOnMount: false });
	const updateMutation = useMutation(updateChatProject(queryClient));
	const singleFlight = useSingleFlight();

	const agents = agentsQuery.data ?? [];
	const invokable = project.invokable_agent_ids.map(
		(id) => agents.find((agent) => agent.id === id) ?? { id, name: "" },
	);
	// Letting an agent invoke another one needs edit access to the other one.
	const available = agents.filter(
		(agent) =>
			agent.id !== project.id &&
			agent.permissions.edit &&
			agent.organization_id === project.organization_id &&
			!project.invokable_agent_ids.includes(agent.id),
	);

	const save = (
		request: Pick<UpdateChatProjectRequest, "invokable_agent_ids">,
	) => {
		singleFlight.run((onSettled) =>
			updateMutation.mutate({ project, request }, { onSettled }),
		);
	};

	return (
		<InvokableAgentsSectionView
			invokable={invokable}
			available={available}
			agentsError={agentsQuery.error}
			onRetryAgents={() => void agentsQuery.refetch()}
			canEdit={project.permissions.edit}
			isSaving={updateMutation.isPending}
			saveError={updateMutation.error}
			onAddAgent={(id) =>
				save({ invokable_agent_ids: [...project.invokable_agent_ids, id] })
			}
			onRemoveAgent={(id) =>
				save({
					invokable_agent_ids: project.invokable_agent_ids.filter(
						(current) => current !== id,
					),
				})
			}
		/>
	);
};

/** An invokable agent. `name` is empty when the viewer cannot read it. */
type InvokableAgent = Pick<ChatProject, "id" | "name"> &
	Partial<Pick<ChatProject, "icon">>;

type InvokableAgentsSectionViewProps = {
	/** The agents this agent may invoke. */
	readonly invokable: readonly InvokableAgent[];
	/** Agents that can still be added. Only the ones the viewer can edit. */
	readonly available: readonly ChatProject[];
	/** Shown with Retry when the agent list fails to load. */
	readonly agentsError: unknown;
	readonly onRetryAgents: () => void;
	readonly canEdit: boolean;
	/**
	 * True while a change saves. Controls stay focusable but ignore input
	 * until it settles.
	 */
	readonly isSaving: boolean;
	readonly saveError: unknown;
	readonly onAddAgent: (id: string) => void;
	readonly onRemoveAgent: (id: string) => void;
};

/** The invokable agents of an agent as card rows, with a searchable add menu. */
export const InvokableAgentsSectionView: React.FC<
	InvokableAgentsSectionViewProps
> = ({
	invokable,
	available,
	agentsError,
	onRetryAgents,
	canEdit,
	isSaving,
	saveError,
	onAddAgent,
	onRemoveAgent,
}) => {
	const removeButtons = useRef(new Map<string, HTMLButtonElement>());
	const addButtonRef = useRef<HTMLButtonElement>(null);

	// The removed row disappears, so focus goes to the next agent's Remove
	// button, or to Add agent after the last row.
	const focusAfterRemoving = (id: string) => {
		const index = invokable.findIndex((agent) => agent.id === id);
		const next = invokable[index + 1];
		const target = next ? removeButtons.current.get(next.id) : undefined;
		(target ?? addButtonRef.current)?.focus();
	};

	return (
		<SettingsSection
			title="Other agents"
			description="Chats in this agent can hand tasks to these agents. You can only add agents you can edit."
		>
			{Boolean(agentsError) && (
				<SectionError error={agentsError} onRetry={onRetryAgents} />
			)}
			{Boolean(saveError) && <SectionSaveError error={saveError} />}
			<SettingsCard>
				{invokable.length === 0 && (
					<SettingsCardEmptyRow>No agents yet.</SettingsCardEmptyRow>
				)}
				{invokable.map((agent) => (
					<InvokableAgentRow
						key={agent.id}
						agent={agent}
						canEdit={canEdit}
						isSaving={isSaving}
						removeButtonRef={(element) => {
							if (element) {
								removeButtons.current.set(agent.id, element);
							}
							return () => {
								removeButtons.current.delete(agent.id);
							};
						}}
						onRemove={() => {
							focusAfterRemoving(agent.id);
							onRemoveAgent(agent.id);
						}}
					/>
				))}
				{canEdit && (
					<AddInvokableAgentRow
						available={available}
						hasInvokable={invokable.length > 0}
						isSaving={isSaving}
						addButtonRef={addButtonRef}
						onAdd={onAddAgent}
					/>
				)}
			</SettingsCard>
		</SettingsSection>
	);
};

type InvokableAgentRowProps = {
	readonly agent: InvokableAgent;
	readonly canEdit: boolean;
	readonly isSaving: boolean;
	readonly removeButtonRef: React.Ref<HTMLButtonElement>;
	readonly onRemove: () => void;
};

const InvokableAgentRow: React.FC<InvokableAgentRowProps> = ({
	agent,
	canEdit,
	isSaving,
	removeButtonRef,
	onRemove,
}) => (
	<SettingsCardRow>
		<SettingsCardRowIcon>
			<ChatProjectIcon
				project={{ icon: agent.icon ?? "" }}
				className="text-content-secondary"
			/>
		</SettingsCardRowIcon>
		<SettingsCardRowText title={agent.name || "Unknown agent"} />
		{canEdit && (
			<Button
				size="icon"
				variant="subtle"
				aria-disabled={isSaving}
				className="aria-disabled:cursor-not-allowed aria-disabled:opacity-50"
				aria-label={`Remove ${agent.name || "unknown agent"}`}
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

type AddInvokableAgentRowProps = {
	readonly available: readonly ChatProject[];
	readonly hasInvokable: boolean;
	readonly isSaving: boolean;
	readonly addButtonRef: React.Ref<HTMLButtonElement>;
	readonly onAdd: (id: string) => void;
};

const AddInvokableAgentRow: React.FC<AddInvokableAgentRowProps> = ({
	available,
	hasInvokable,
	isSaving,
	addButtonRef,
	onAdd,
}) => {
	let emptyLabel = "There are no other agents you can edit.";
	if (available.length > 0) {
		emptyLabel = "No agents found.";
	} else if (hasInvokable) {
		emptyLabel = "Every agent you can edit is added.";
	}
	return (
		<Combobox>
			<ComboboxTrigger asChild>
				<SettingsCardAddRow
					ref={addButtonRef}
					label="Add agent"
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
				<ComboboxInput placeholder="Search agents" />
				<ComboboxList>
					{available.map((agent) => (
						<ComboboxItem
							key={agent.id}
							value={agent.id}
							keywords={[agent.name, agent.description]}
							onSelect={() => onAdd(agent.id)}
						>
							<SettingsCardRowIcon>
								<ChatProjectIcon
									project={agent}
									className="text-content-secondary"
								/>
							</SettingsCardRowIcon>
							{agent.name}
						</ComboboxItem>
					))}
				</ComboboxList>
				<ComboboxEmpty>{emptyLabel}</ComboboxEmpty>
			</ComboboxContent>
		</Combobox>
	);
};
