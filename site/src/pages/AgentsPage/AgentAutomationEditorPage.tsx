import { ChevronRightIcon, PlayIcon, Trash2Icon } from "lucide-react";
import { useRef, useState } from "react";
import { flushSync } from "react-dom";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { Link as RouterLink, useNavigate, useParams } from "react-router";
import { toast } from "sonner";
import { getErrorDetail, getErrorMessage, getErrorStatus } from "#/api/errors";
import { checkAuthorization } from "#/api/queries/authCheck";
import {
	chatAutomations,
	createChatAutomation,
	deleteChatAutomation,
	fetchSkippedRun,
	rotateChatAutomationSecret,
	runChatAutomation,
	updateChatAutomation,
	webhookOrigin,
	webhookPublishEndpoint,
} from "#/api/queries/chatAutomations";
import type {
	ChatAutomation,
	CreateChatAutomationRequest,
	UpdateChatAutomationRequest,
} from "#/api/typesGenerated";
import { Alert, AlertDescription, AlertTitle } from "#/components/Alert/Alert";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Button } from "#/components/Button/Button";
import { Loader } from "#/components/Loader/Loader";
import { ScrollArea } from "#/components/ScrollArea/ScrollArea";
import { Spinner } from "#/components/Spinner/Spinner";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import NotFoundPage from "#/pages/NotFoundPage/NotFoundPage";
import {
	AutomationsPageHeader,
	useSelectedAutomationsOrganization,
} from "./AgentAutomationsPage";
import { AutomationDeleteDialog } from "./components/Automations/AutomationDeleteDialog";
import {
	AutomationEditorForm,
	AutomationRunsAsCard,
	type SecretGuard,
} from "./components/Automations/AutomationEditorForm";
import {
	AutomationLastRun,
	skippedRunMessage,
} from "./components/Automations/AutomationLastRun";
import {
	AutomationEnabledSwitch,
	AutomationNextRun,
	AutomationPausedBadge,
	runNowBlockedReason,
} from "./components/Automations/AutomationStatus";
import { AutomationSummary } from "./components/Automations/AutomationSummary";
import { AutomationWebhookSecretDialog } from "./components/Automations/AutomationWebhookSecretDialog";
import {
	AUTOMATIONS_PATH,
	useAutomationsEnabled,
} from "./components/Automations/automationsFlag";

const backToList = { pathname: AUTOMATIONS_PATH, label: "Automations" };

/**
 * Creates an automation at /agents/automations/new, and edits one at
 * /agents/automations/:automationId. Viewers who do not own the automation
 * see a read-only summary on the same route.
 */
const AgentAutomationEditorPage: React.FC = () => {
	return useAutomationsEnabled() ? (
		<AutomationEditorRoute />
	) : (
		<div className="flex min-h-0 flex-1 flex-col">
			<AutomationsPageHeader />
			<div className="min-h-0 flex-1">
				<NotFoundPage />
			</div>
		</div>
	);
};

type BreadcrumbProps = {
	current: string;
};

const Breadcrumb: React.FC<BreadcrumbProps> = ({ current }) => (
	<nav aria-label="Breadcrumb" className="min-w-0">
		<ol className="m-0 flex list-none items-center gap-1 p-0 text-sm">
			<li>
				<RouterLink
					to={AUTOMATIONS_PATH}
					className="text-content-secondary no-underline hover:text-content-primary"
				>
					Automations
				</RouterLink>
			</li>
			<li aria-hidden className="flex text-content-secondary">
				<ChevronRightIcon className="size-3.5" />
			</li>
			<li
				aria-current="page"
				className="min-w-0 truncate font-medium text-content-primary"
			>
				{current}
			</li>
		</ol>
	</nav>
);

const AutomationEditorRoute: React.FC = () => {
	const { automationId } = useParams();
	const { selectedOrg } = useSelectedAutomationsOrganization();
	const organizationId = selectedOrg?.id ?? "";
	const isCreate = automationId === undefined;
	const automationsQuery = useQuery({
		...chatAutomations(organizationId),
		enabled: Boolean(organizationId) && !isCreate,
		// Keeps the next run and the enabled state current.
		refetchInterval: 60_000,
	});
	const automation = automationsQuery.data?.find(
		({ id }) => id === automationId,
	);

	let content: React.ReactNode;
	if (!isCreate && automationsQuery.isLoading) {
		content = <Loader />;
	} else if (!isCreate && !automation) {
		content = automationsQuery.error ? (
			<ErrorAlert error={automationsQuery.error} />
		) : (
			<div className="flex flex-col items-start gap-4">
				<h1 className="m-0 text-xl font-semibold text-content-primary">
					Automation not found
				</h1>
				<p className="m-0 text-sm text-content-secondary">
					It does not exist in{" "}
					{selectedOrg?.display_name ||
						selectedOrg?.name ||
						"this organization"}
					, or you cannot see it. If it belongs to another organization, pick
					that organization on the Automations page.
				</p>
				<Button asChild variant="outline" size="sm">
					<RouterLink to={AUTOMATIONS_PATH}>Back to automations</RouterLink>
				</Button>
			</div>
		);
	} else {
		return (
			<AutomationEditor
				organizationId={organizationId}
				automation={automation}
			/>
		);
	}

	return (
		<ScrollArea className="min-h-0 flex-1" viewportClassName="[&>div]:block!">
			<AutomationsPageHeader backTo={backToList} />
			<div className="px-4 pb-8 pt-2 sm:pt-6">
				<div className="mx-auto flex w-full max-w-3xl flex-col gap-6">
					<Breadcrumb current={isCreate ? "New automation" : "Not found"} />
					{content}
				</div>
			</div>
		</ScrollArea>
	);
};

type AutomationEditorProps = {
	organizationId: string;
	automation: ChatAutomation | undefined;
};

const AutomationEditor: React.FC<AutomationEditorProps> = ({
	organizationId,
	automation,
}) => {
	const { buildInfo, experiments } = useDashboard();
	const { user } = useAuthenticated();
	const queryClient = useQueryClient();
	const navigate = useNavigate();
	const origin = webhookOrigin(buildInfo.dashboard_url, window.location.origin);
	const isOwner = !automation || automation.owner_id === user.id;
	const ownerName = automation?.owner
		? automation.owner.name || automation.owner.username
		: isOwner
			? user.name || user.username
			: "the owner";
	// The only copy of a new webhook secret. Never cache or persist it.
	const [webhookSecret, setWebhookSecret] = useState<{
		automationId: string;
		secret: string;
	}>();
	// The Rotate secret button during a rotation.
	const secretReturnFocusRef = useRef<HTMLElement | null>(null);
	// Turns the leave prompt off right before a deliberate navigation.
	const [allowLeave, setAllowLeave] = useState(false);
	const [runError, setRunError] = useState<unknown>();
	const [confirmingDelete, setConfirmingDelete] = useState(false);
	// The same rule as the list: the owner, or a viewer who may delete any
	// automation in the organization.
	const deleteAnyQuery = useQuery({
		...checkAuthorization({
			checks: {
				canDeleteAny: {
					object: {
						resource_type: "chat_automation",
						organization_id: organizationId,
					},
					action: "delete",
				},
			},
		}),
		enabled: Boolean(automation) && !isOwner && Boolean(organizationId),
	});
	const canDelete =
		Boolean(automation) &&
		(isOwner || Boolean(deleteAnyQuery.data?.canDeleteAny));
	const deleteMutation = useMutation(
		deleteChatAutomation(queryClient, organizationId),
	);

	// The router's blocker reads the prompt state of the last render, so the
	// render that turns the prompt off must commit before navigating.
	const leaveToList = () => {
		flushSync(() => {
			setWebhookSecret(undefined);
			setAllowLeave(true);
		});
		navigate(AUTOMATIONS_PATH);
	};

	const createMutation = useMutation(
		createChatAutomation(queryClient, organizationId, (automationId, secret) =>
			setWebhookSecret({ automationId, secret }),
		),
	);
	const disableMutation = useMutation(
		updateChatAutomation(queryClient, organizationId),
	);
	const editMutation = useMutation(
		updateChatAutomation(queryClient, organizationId),
	);
	const toggleMutation = useMutation(
		updateChatAutomation(queryClient, organizationId),
	);
	const rotateMutation = useMutation(
		rotateChatAutomationSecret(
			queryClient,
			organizationId,
			(automationId, secret) => setWebhookSecret({ automationId, secret }),
		),
	);
	const runMutation = useMutation(
		runChatAutomation(queryClient, organizationId),
	);

	// Leaving mid-request, or before copying, loses the one-time secret.
	let secretGuard: SecretGuard | undefined;
	if (webhookSecret) {
		secretGuard = "shown";
	} else if (
		rotateMutation.isPending ||
		(createMutation.isPending && createMutation.variables?.kind === "webhook")
	) {
		secretGuard = "pending";
	}

	const handleCreate = (req: CreateChatAutomationRequest, enable: boolean) => {
		createMutation.mutate(req, {
			onSuccess: ({ automation: created }) => {
				const finish = () => {
					toast.success(
						enable
							? `Created ${created.name}.`
							: `Created ${created.name}. It is turned off.`,
					);
					// A webhook stays until the user closes its secret dialog.
					if (created.kind === "schedule") {
						leaveToList();
					}
				};
				if (enable) {
					finish();
					return;
				}
				// The create API has no enabled field, so Save as disabled
				// turns the new automation off right after it is created.
				disableMutation.mutate(
					{ automationId: created.id, req: { enabled: false } },
					{
						onSuccess: finish,
						onError: (error) => {
							toast.error(
								`Created ${created.name}, but could not turn it off.`,
								{ description: getErrorMessage(error, "") },
							);
							if (created.kind === "schedule") {
								leaveToList();
							}
						},
					},
				);
			},
		});
	};

	const handleUpdate = (req: UpdateChatAutomationRequest) => {
		if (!automation) {
			return;
		}
		rotateMutation.reset();
		editMutation.mutate(
			{ automationId: automation.id, req },
			{
				onSuccess: (updated) => {
					toast.success(`Saved ${updated.name}.`);
					leaveToList();
				},
			},
		);
	};

	const handleToggle = (enabled: boolean) => {
		if (!automation) {
			return;
		}
		toggleMutation.mutate(
			{ automationId: automation.id, req: { enabled } },
			{
				onSuccess: (updated) =>
					toast.success(
						updated.enabled || isOwner
							? `${updated.name} is ${updated.enabled ? "on" : "off"}.`
							: // Only the owner can undo this, so say so right away.
								`${updated.name} is off. Only ${ownerName} can turn it back on.`,
					),
				onError: (error) =>
					toast.error(
						getErrorMessage(error, `Could not update ${automation.name}.`),
						{ description: getErrorDetail(error) },
					),
			},
		);
	};

	const handleRunNow = () => {
		if (!automation) {
			return;
		}
		setRunError(undefined);
		runMutation.mutate(automation.id, {
			onSuccess: ({ chat_id }) =>
				toast.success(`${automation.name} accepted the run.`, {
					action: {
						label: "Open chat",
						onClick: () => navigate(`/agents/${chat_id}`),
					},
				}),
			onError: (error) => {
				void fetchSkippedRun(queryClient, organizationId, automation)
					.catch(() => undefined)
					.then((skipped) => {
						if (skipped) {
							toast.message(skippedRunMessage(automation.name, skipped));
							return;
						}
						setRunError(error);
					});
			},
		});
	};

	const handleDelete = () => {
		if (!automation) {
			return;
		}
		// The delete drops the automation from the cache, so the route renders
		// "not found" and unmounts this editor before mutate() callbacks run.
		// mutateAsync settles regardless, so the toast and redirect still happen.
		deleteMutation.mutateAsync(automation.id).then(
			() => {
				toast.success(`Deleted ${automation.name}.`);
				leaveToList();
			},
			(error: unknown) => {
				// Someone else deleted it first, so there is nothing to retry.
				// Other errors stay in the dialog through deleteMutation.error.
				if (getErrorStatus(error) === 404) {
					toast.message(`${automation.name} was already deleted.`);
					leaveToList();
				}
			},
		);
	};

	const runNowBlocked = automation ? runNowBlockedReason(automation) : "";
	const runNowButton = automation &&
		isOwner &&
		automation.kind === "schedule" && (
			<RunNowPageButton
				blockedReason={runNowBlocked}
				isRunning={runMutation.isPending}
				onRunNow={handleRunNow}
			/>
		);
	const title = automation
		? isOwner
			? "Edit automation"
			: automation.name
		: "New automation";

	return (
		<ScrollArea className="min-h-0 flex-1" viewportClassName="[&>div]:block!">
			<AutomationsPageHeader backTo={backToList} />
			<div className="px-4 pb-8 pt-2 sm:pt-6">
				<div className="mx-auto flex w-full max-w-3xl flex-col gap-6">
					{/* In the content column, not the top bar, so the breadcrumb and
					    actions line up with the title at every width. */}
					<div className="flex min-h-9 flex-wrap items-center justify-between gap-3">
						<Breadcrumb current={automation?.name ?? "New automation"} />
						{(runNowButton || canDelete) && (
							<div className="flex items-center gap-2">
								{runNowButton}
								{canDelete && automation && (
									<Tooltip>
										<TooltipTrigger asChild>
											<Button
												size="icon"
												variant="outline"
												aria-label={`Delete ${automation.name}`}
												onClick={() => {
													deleteMutation.reset();
													setConfirmingDelete(true);
												}}
											>
												<Trash2Icon />
											</Button>
										</TooltipTrigger>
										<TooltipContent>Delete</TooltipContent>
									</Tooltip>
								)}
							</div>
						)}
					</div>
					<header className="flex flex-wrap items-start justify-between gap-4">
						<div className="flex min-w-0 flex-col gap-1">
							<h1 className="m-0 text-xl font-semibold text-content-primary wrap-anywhere">
								{title}
							</h1>
							{automation && (
								<>
									<span className="text-sm text-content-secondary">
										Next run: <AutomationNextRun automation={automation} />
									</span>
									<AutomationLastRun
										lastRun={automation.last_run}
										showHint={isOwner}
										className="text-sm"
									/>
								</>
							)}
						</div>
						{automation && (
							<div className="flex flex-wrap items-center gap-3">
								<AutomationPausedBadge automation={automation} />
								<AutomationEnabledSwitch
									automation={automation}
									isOwner={isOwner}
									isUpdating={toggleMutation.isPending}
									onToggle={handleToggle}
									showLabel
								/>
							</div>
						)}
					</header>
					{Boolean(runError) && (
						<Alert
							severity="error"
							prominent
							dismissible
							onDismiss={() => setRunError(undefined)}
						>
							<AlertTitle>Could not run {automation?.name}</AlertTitle>
							<AlertDescription>
								{getErrorMessage(runError, "The run did not start.")}
								{getErrorDetail(runError) && (
									<span className="block">{getErrorDetail(runError)}</span>
								)}
							</AlertDescription>
						</Alert>
					)}
					{automation && !isOwner ? (
						<>
							<p className="m-0 text-sm text-content-secondary">
								{automation.enabled
									? `Only ${ownerName} can change this automation. You can turn it off, but only ${ownerName} can turn it back on.`
									: `Only ${ownerName} can change this automation or turn it back on.`}
							</p>
							<AutomationSummary automation={automation} origin={origin} />
							<AutomationRunsAsCard
								runsAsName={ownerName}
								isWebhook={automation.kind === "webhook"}
							/>
							<div className="flex justify-end border-0 border-t border-solid border-border pt-6">
								<Button asChild variant="outline">
									<RouterLink to={AUTOMATIONS_PATH}>
										Back to automations
									</RouterLink>
								</Button>
							</div>
						</>
					) : (
						<AutomationEditorForm
							organizationId={organizationId}
							automation={automation}
							projectsEnabled={experiments.includes("chat-projects")}
							origin={origin}
							runsAsName={ownerName}
							error={automation ? editMutation.error : createMutation.error}
							isSubmitting={
								automation
									? editMutation.isPending
									: createMutation.isPending || disableMutation.isPending
							}
							onCreate={handleCreate}
							onUpdate={handleUpdate}
							rotateSecretError={rotateMutation.error}
							isRotatingSecret={rotateMutation.isPending}
							onRotateSecret={(rotateButton) => {
								if (!automation) {
									return;
								}
								// Each save or rotate replaces the other's error so
								// alerts do not stack.
								editMutation.reset();
								secretReturnFocusRef.current = rotateButton;
								rotateMutation.mutate(automation.id);
							}}
							onCancel={() => navigate(AUTOMATIONS_PATH)}
							secretGuard={secretGuard}
							allowLeave={allowLeave}
						/>
					)}
				</div>
			</div>
			{automation && confirmingDelete && (
				<AutomationDeleteDialog
					automation={automation}
					isOwner={isOwner}
					isDeleting={deleteMutation.isPending}
					error={deleteMutation.error}
					onConfirm={handleDelete}
					onClose={() => setConfirmingDelete(false)}
				/>
			)}
			{webhookSecret && (
				<AutomationWebhookSecretDialog
					endpoint={webhookPublishEndpoint(origin, webhookSecret.automationId)}
					secret={webhookSecret.secret}
					returnFocusRef={secretReturnFocusRef}
					onClose={() => {
						if (automation) {
							setWebhookSecret(undefined);
							return;
						}
						leaveToList();
					}}
				/>
			)}
		</ScrollArea>
	);
};

type RunNowPageButtonProps = {
	/** Why the server would refuse a run; empty or undefined when it would not. */
	blockedReason: string | undefined;
	isRunning: boolean;
	onRunNow: () => void;
};

const RunNowPageButton: React.FC<RunNowPageButtonProps> = ({
	blockedReason,
	isRunning,
	onRunNow,
}) => {
	const button = (
		<Button
			size="sm"
			variant="outline"
			disabled={isRunning}
			aria-disabled={Boolean(blockedReason)}
			className="aria-disabled:cursor-not-allowed aria-disabled:opacity-50"
			onClick={() => {
				if (!blockedReason) {
					onRunNow();
				}
			}}
		>
			<Spinner loading={isRunning}>
				<PlayIcon />
			</Spinner>
			Run now
		</Button>
	);
	if (!blockedReason) {
		return button;
	}
	return (
		<Tooltip>
			<TooltipTrigger asChild>{button}</TooltipTrigger>
			<TooltipContent className="max-w-xs">{blockedReason}</TooltipContent>
		</Tooltip>
	);
};

export default AgentAutomationEditorPage;
