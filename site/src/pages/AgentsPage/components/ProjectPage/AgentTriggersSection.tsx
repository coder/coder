import { TriangleAlertIcon } from "lucide-react";
import { Fragment, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { useNavigate } from "react-router";
import { getErrorMessageWithValidations } from "#/api/errors";
import {
	chatProjectTriggers,
	chatProjectTriggersKey,
	createChatProjectTrigger,
	deleteChatProjectTrigger,
	regenerateChatProjectTriggerWebhookURL,
	runChatProjectTrigger,
	updateChatProjectTrigger,
} from "#/api/queries/chatProjects";
import type {
	ChatProject,
	ChatProjectTrigger,
	ChatProjectTriggerSource,
	CreateChatProjectTriggerResponse,
	UpdateChatProjectTriggerRequest,
} from "#/api/typesGenerated";
import { ConfirmDialog } from "#/components/Dialog/ConfirmDialog/ConfirmDialog";
import { DeleteDialog } from "#/components/Dialog/DeleteDialog/DeleteDialog";
import { toast } from "#/components/Toaster/toast";
import { useTime } from "#/hooks/useTime";
import { getPreferredTimezone } from "#/utils/timeZones";
import { buildAgentChatPath } from "../../utils/navigation";
import {
	SectionError,
	SectionLoadState,
	SectionSaveError,
	SettingsCard,
	SettingsCardEmptyRow,
	SettingsSection,
} from "./ProjectSection";
import { TriggerAddMenu } from "./TriggerAddMenu";
import { TriggerRow } from "./TriggerRow";
import { TriggerScheduleDialog } from "./TriggerScheduleDialog";
import type { TriggerUpdateCallbacks } from "./TriggerSentenceChip";
import { TriggerWebhookCallout } from "./TriggerWebhookCallout";
import {
	suggestTriggerName,
	triggerSources,
	withSuggestedName,
} from "./triggerCatalog";

type AgentTriggersSectionProps = {
	readonly project: ChatProject;
};

/**
 * The triggers card of an agent: each trigger starts a chat when GitHub,
 * Slack, or any other service calls its webhook, or on a schedule.
 */
export const AgentTriggersSection: React.FC<AgentTriggersSectionProps> = ({
	project,
}) => {
	const queryClient = useQueryClient();
	const navigate = useNavigate();
	const triggersQuery = useQuery(chatProjectTriggers(queryClient, project));
	const createMutation = useMutation(
		createChatProjectTrigger(queryClient, project),
	);
	const updateMutation = useMutation(
		updateChatProjectTrigger(queryClient, project),
	);
	const regenerateMutation = useMutation(
		regenerateChatProjectTriggerWebhookURL(project),
	);
	const deleteMutation = useMutation(
		deleteChatProjectTrigger(queryClient, project),
	);
	const runMutation = useMutation(runChatProjectTrigger(queryClient, project));
	// The latest failure of an add, edit, new webhook URL, run, or delete.
	const [saveError, setSaveError] = useState<unknown>();
	const [isAddingSchedule, setIsAddingSchedule] = useState(false);
	const [scheduleError, setScheduleError] = useState<unknown>();
	const [revealed, setRevealed] = useState<CreateChatProjectTriggerResponse>();
	const [triggerToRegenerate, setTriggerToRegenerate] =
		useState<ChatProjectTrigger>();
	const [triggerToDelete, setTriggerToDelete] = useState<ChatProjectTrigger>();
	// Where focus goes when the open dialog closes.
	const restoreRegenerateFocus = useRef<() => void>(undefined);
	const restoreDeleteFocus = useRef<(deleted: boolean) => void>(undefined);

	const closeRegenerateDialog = () => {
		setTriggerToRegenerate(undefined);
		const restore = restoreRegenerateFocus.current;
		if (restore) {
			requestAnimationFrame(restore);
		}
	};
	const closeDeleteDialog = (deleted: boolean) => {
		setTriggerToDelete(undefined);
		const restore = restoreDeleteFocus.current;
		if (restore) {
			requestAnimationFrame(() => restore(deleted));
		}
	};

	return (
		<>
			<AgentTriggersSectionView
				triggers={triggersQuery.data}
				project={project}
				canManage={project.permissions.manage_triggers}
				error={triggersQuery.error}
				onRetry={() => void triggersQuery.refetch()}
				saveError={saveError}
				isCreating={createMutation.isPending}
				revealed={revealed}
				onDismissRevealed={() => setRevealed(undefined)}
				onCreate={(source, events) => {
					setSaveError(undefined);
					createMutation.mutate(
						{
							name: suggestTriggerName({
								source,
								events,
								repository: "",
								channel: "",
							}),
							prompt: "",
							source,
							events,
						},
						{
							onSuccess: setRevealed,
							onError: setSaveError,
						},
					);
				}}
				onAddSchedule={() => {
					setScheduleError(undefined);
					setIsAddingSchedule(true);
				}}
				onUpdate={(trigger, request, callbacks) =>
					// mutateAsync reports every edit; mutate's callbacks only
					// run for the latest of several queued edits.
					updateMutation
						.mutateAsync({
							triggerId: trigger.id,
							request: withSuggestedName(trigger, request),
						})
						.then(
							() => {
								setSaveError(undefined);
								callbacks?.onSuccess?.();
							},
							(error: unknown) => {
								const message = getErrorMessageWithValidations(
									error,
									"Failed to save trigger.",
								);
								setSaveError(error);
								callbacks?.onError?.(message);
							},
						)
				}
				onRegenerateURL={(trigger, restoreFocus) => {
					restoreRegenerateFocus.current = restoreFocus;
					setTriggerToRegenerate(trigger);
				}}
				runningTriggerId={
					runMutation.isPending ? runMutation.variables : undefined
				}
				onRunNow={(trigger) => {
					// A second click while a run starts would start another chat.
					if (runMutation.isPending) {
						return;
					}
					setSaveError(undefined);
					runMutation.mutate(trigger.id, {
						onSuccess: (response) =>
							toast.success(
								response.continued
									? `Sent a run of "${trigger.name}" to its chat.`
									: `Started a chat from "${trigger.name}".`,
								{
									action: {
										label: "Open chat",
										onClick: () =>
											navigate(
												buildAgentChatPath({ chatId: response.chat_id }),
											),
									},
								},
							),
						onError: setSaveError,
					});
				}}
				onDelete={(trigger, restoreFocus) => {
					restoreDeleteFocus.current = restoreFocus;
					setTriggerToDelete(trigger);
				}}
			/>
			<ConfirmDialog
				open={triggerToRegenerate !== undefined}
				type="delete"
				title="Get a new webhook URL?"
				description={
					triggerToRegenerate &&
					`The current URL of "${triggerToRegenerate.name}" stops working right away. Paste the new one into ${triggerSources[triggerToRegenerate.source].title} to keep the trigger running.`
				}
				confirmText="Get new webhook URL"
				confirmLoading={regenerateMutation.isPending}
				onClose={closeRegenerateDialog}
				onConfirm={() => {
					if (!triggerToRegenerate) {
						return;
					}
					regenerateMutation.mutate(triggerToRegenerate.id, {
						onSuccess: (response) => {
							setSaveError(undefined);
							setRevealed(response);
							closeRegenerateDialog();
						},
						onError: setSaveError,
					});
				}}
			/>
			<DeleteDialog
				isOpen={triggerToDelete !== undefined}
				entity="trigger"
				name={triggerToDelete?.name ?? ""}
				info={
					triggerToDelete && triggerSources[triggerToDelete.source].deleteInfo
				}
				confirmLoading={deleteMutation.isPending}
				onCancel={() => closeDeleteDialog(false)}
				onConfirm={() => {
					if (!triggerToDelete) {
						return;
					}
					deleteMutation.mutate(triggerToDelete.id, {
						onSuccess: () => {
							setSaveError(undefined);
							closeDeleteDialog(true);
							if (revealed?.trigger.id === triggerToDelete.id) {
								setRevealed(undefined);
							}
						},
						onError: setSaveError,
					});
				}}
			/>
			{isAddingSchedule && (
				<TriggerScheduleDialog
					agentName={project.name}
					isCreating={createMutation.isPending}
					error={scheduleError}
					onScheduleChange={() => setScheduleError(undefined)}
					onClose={() => setIsAddingSchedule(false)}
					onCreate={({ schedule, prompt }) => {
						setScheduleError(undefined);
						createMutation.mutate(
							{
								name: suggestTriggerName({
									source: "schedule",
									events: [],
									schedule,
								}),
								prompt,
								source: "schedule",
								schedule,
							},
							{
								onSuccess: ({ trigger }) => {
									setSaveError(undefined);
									setIsAddingSchedule(false);
									// Shows the new row before a refetch includes it. `revealed`
									// is only for one-time webhook URLs.
									queryClient.setQueryData<ChatProjectTrigger[]>(
										chatProjectTriggersKey(project.id),
										(triggers) =>
											triggers?.some((t) => t.id === trigger.id)
												? triggers
												: triggers && [...triggers, trigger],
									);
								},
								onError: setScheduleError,
							},
						);
					}}
				/>
			)}
		</>
	);
};

type AgentTriggersSectionViewProps = {
	/** `undefined` until the first load finishes. */
	readonly triggers: readonly ChatProjectTrigger[] | undefined;
	readonly project: ChatProject;
	/** False shows the triggers read-only. */
	readonly canManage: boolean;
	readonly error: unknown;
	readonly onRetry: () => void;
	/** The latest failed change, shown under the section header. */
	readonly saveError: unknown;
	readonly isCreating: boolean;
	/** A webhook URL to show once, under the trigger it belongs to. */
	readonly revealed: CreateChatProjectTriggerResponse | undefined;
	readonly onDismissRevealed: () => void;
	readonly onCreate: (
		source: ChatProjectTriggerSource,
		events: readonly string[],
	) => void;
	/** Opens the form for a new schedule trigger. */
	readonly onAddSchedule: () => void;
	readonly onUpdate: (
		trigger: ChatProjectTrigger,
		request: UpdateChatProjectTriggerRequest,
		callbacks?: TriggerUpdateCallbacks,
	) => void;
	/** `restoreFocus` is where focus goes when the confirmation closes. */
	readonly onRegenerateURL: (
		trigger: ChatProjectTrigger,
		restoreFocus: (() => void) | undefined,
	) => void;
	readonly onRunNow: (trigger: ChatProjectTrigger) => void;
	/** The schedule trigger whose Run now is starting. */
	readonly runningTriggerId?: string;
	/**
	 * The time relative times count from. Unset uses the current time,
	 * updated every 30 seconds; stories pin it.
	 */
	readonly now?: Date;
	/** The viewer's time zone, which exact times show in. Unset uses the browser's. */
	readonly timeZone?: string;
	/**
	 * `restoreFocus` takes whether the trigger was deleted and moves focus
	 * once the dialog closes.
	 */
	readonly onDelete: (
		trigger: ChatProjectTrigger,
		restoreFocus: (deleted: boolean) => void,
	) => void;
};

/** The "Triggers" label, a card with one sentence row per trigger, and warnings. */
export const AgentTriggersSectionView: React.FC<
	AgentTriggersSectionViewProps
> = ({
	triggers,
	project,
	canManage,
	error,
	onRetry,
	saveError,
	isCreating,
	revealed,
	onDismissRevealed,
	onCreate,
	onAddSchedule,
	onUpdate,
	onRegenerateURL,
	onRunNow,
	runningTriggerId,
	now,
	timeZone,
	onDelete,
}) => {
	const addTriggerRef = useRef<HTMLButtonElement>(null);
	const liveNow = useTime(() => new Date(), {
		interval: 30_000,
		disabled: now !== undefined,
	});
	const currentTime = now ?? liveNow;
	const viewerTimeZone = timeZone ?? getPreferredTimezone();
	// A new trigger can be revealed before the list refetch includes it.
	const rows =
		triggers && revealed && !triggers.some((t) => t.id === revealed.trigger.id)
			? [...triggers, revealed.trigger]
			: triggers;
	return (
		<SettingsSection
			title="Triggers"
			description={
				canManage
					? "Start a chat when a pull request opens, someone mentions your Slack app, a service calls a webhook, or on a schedule."
					: "Triggers start chats as the agent's user. Only the owner can add or change triggers."
			}
		>
			{rows === undefined ? (
				<SectionLoadState error={error} onRetry={onRetry} />
			) : (
				<>
					{Boolean(error) && <SectionError error={error} onRetry={onRetry} />}
					{Boolean(saveError) && <SectionSaveError error={saveError} />}
					<SettingsCard>
						{rows.length === 0 && (
							<SettingsCardEmptyRow>No triggers yet.</SettingsCardEmptyRow>
						)}
						{rows.map((trigger) => (
							<Fragment key={trigger.id}>
								<TriggerRow
									trigger={trigger}
									project={project}
									canManage={canManage}
									isRunning={runningTriggerId === trigger.id}
									now={currentTime}
									timeZone={viewerTimeZone}
									onUpdate={onUpdate}
									onRegenerateURL={onRegenerateURL}
									onRunNow={onRunNow}
									onDelete={(trigger, focusActions) =>
										// A deleted row takes its menu button with it.
										onDelete(trigger, (deleted) =>
											deleted ? addTriggerRef.current?.focus() : focusActions(),
										)
									}
								/>
								{revealed?.trigger.id === trigger.id && (
									<TriggerWebhookCallout
										trigger={trigger}
										webhookURL={revealed.webhook_url}
										onUpdate={(request, callbacks) =>
											onUpdate(trigger, request, callbacks)
										}
										onDismiss={onDismissRevealed}
									/>
								)}
							</Fragment>
						))}
						{canManage && (
							<TriggerAddMenu
								ref={addTriggerRef}
								isCreating={isCreating}
								onCreate={onCreate}
								onAddSchedule={onAddSchedule}
							/>
						)}
					</SettingsCard>
					{canManage && (
						<TriggerWarnings
							triggers={rows}
							revealedTriggerId={revealed?.trigger.id}
							onRegenerateURL={onRegenerateURL}
						/>
					)}
				</>
			)}
		</SettingsSection>
	);
};

type TriggerWarningsProps = {
	readonly triggers: readonly ChatProjectTrigger[];
	/** The trigger whose new webhook URL is on screen; it gets no setup warning. */
	readonly revealedTriggerId: string | undefined;
	readonly onRegenerateURL: AgentTriggersSectionViewProps["onRegenerateURL"];
};

/** Amber notes under the card for enabled webhook triggers that may not work yet. */
const TriggerWarnings: React.FC<TriggerWarningsProps> = ({
	triggers,
	revealedTriggerId,
	onRegenerateURL,
}) => {
	const pending = triggers.filter(
		(trigger) =>
			trigger.enabled &&
			trigger.source !== "schedule" &&
			!trigger.last_triggered_at &&
			!trigger.last_delivery &&
			trigger.id !== revealedTriggerId,
	);
	if (pending.length === 0) {
		return null;
	}
	return (
		<ul className="m-0 flex list-none flex-col gap-1.5 p-0 text-xs text-content-warning">
			{pending.map((trigger) => (
				<Fragment key={trigger.id}>
					{trigger.source === "slack" && trigger.channel && (
						<TriggerWarning name={trigger.name}>
							Invite your Slack app to the channel, or it won't receive messages
							there.
						</TriggerWarning>
					)}
					<TriggerWarning name={trigger.name}>
						Not set up yet. Nothing has called its webhook URL.{" "}
						<button
							type="button"
							className="cursor-pointer border-0 bg-transparent p-0 font-medium text-content-warning underline underline-offset-2 hover:text-content-primary focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-content-link"
							onClick={() => onRegenerateURL(trigger, undefined)}
						>
							Get new webhook URL
						</button>
					</TriggerWarning>
				</Fragment>
			))}
		</ul>
	);
};

type TriggerWarningProps = {
	readonly name: string;
	readonly children: React.ReactNode;
};

const TriggerWarning: React.FC<TriggerWarningProps> = ({ name, children }) => (
	<li className="flex items-start gap-2">
		<TriangleAlertIcon aria-hidden className="mt-px size-icon-xs shrink-0" />
		<span>
			<span className="font-medium">{name}:</span> {children}
		</span>
	</li>
);
