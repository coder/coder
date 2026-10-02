import { useRef, useState } from "react";
import {
	useInfiniteQuery,
	useMutation,
	useQuery,
	useQueryClient,
} from "react-query";
import { useLocation } from "react-router";
import { toast } from "sonner";
import { getErrorDetail, getErrorMessage, getErrorStatus } from "#/api/errors";
import {
	automationChats,
	chatAutomations,
	createChatAutomation,
	deleteChatAutomation,
	rotateChatAutomationSecret,
	runChatAutomation,
	updateChatAutomation,
	webhookPublishEndpoint,
} from "#/api/queries/chatAutomations";
import type {
	ChatAutomation,
	Organization,
	UpdateChatAutomationRequest,
} from "#/api/typesGenerated";
import { ConfirmDialog } from "#/components/Dialog/ConfirmDialog/ConfirmDialog";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { useUnsavedChangesPrompt } from "#/hooks/useUnsavedChangesPrompt";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import NotFoundPage from "#/pages/NotFoundPage/NotFoundPage";
import {
	AgentAutomationsPageView,
	type AutomationRunError,
} from "./AgentAutomationsPageView";
import { selectedOrganizationIdStorageKey } from "./components/AgentCreateForm";
import { AgentPageHeader } from "./components/AgentPageHeader";
import { AutomationEditorDialog } from "./components/Automations/AutomationEditorDialog";
import { AutomationWebhookSecretDialog } from "./components/Automations/AutomationWebhookSecretDialog";
import { useAutomationsEnabled } from "./components/Automations/automationsFlag";
import { CompactOrgSelector } from "./components/ChatElements/CompactOrgSelector";
import { normalizeLocationSearch } from "./components/ChatsSidebar/locationSearch";

const AgentAutomationsPage: React.FC = () => {
	return useAutomationsEnabled() ? (
		<AutomationsList />
	) : (
		<div className="flex min-h-0 flex-1 flex-col">
			<AutomationsPageHeader />
			<div className="min-h-0 flex-1">
				<NotFoundPage />
			</div>
		</div>
	);
};

/** Keeps the sidebar's search filters on the mobile link back to Agents. */
const AutomationsPageHeader: React.FC = () => {
	const location = useLocation();
	return (
		<AgentPageHeader
			mobileBack={{
				to: {
					pathname: "/agents",
					search: normalizeLocationSearch(location.search),
				},
				label: "Agents",
			}}
		/>
	);
};

type EditorState =
	| { mode: "create" }
	| { mode: "edit"; automation: ChatAutomation };

const AutomationsList: React.FC = () => {
	const { buildInfo, organizations, showOrganizations } = useDashboard();
	const queryClient = useQueryClient();
	const { user } = useAuthenticated();
	// Senders must reach the configured URL, not the address this tab uses.
	const webhookOrigin = new URL(buildInfo.dashboard_url).origin;
	const [selectedOrgId, setSelectedOrgId] = useState(() =>
		localStorage.getItem(selectedOrganizationIdStorageKey),
	);
	const selectedOrg =
		organizations.find((org) => org.id === selectedOrgId) ??
		organizations.find((org) => org.is_default) ??
		organizations[0];
	const organizationId = selectedOrg?.id ?? "";

	// Holds the organization of the run, so a failure that arrives after the
	// viewer switched organizations does not show on the new one.
	const [runError, setRunError] = useState<
		AutomationRunError & { organizationId: string }
	>();
	const [chatsAutomation, setChatsAutomation] = useState<ChatAutomation>();
	const [deleteTarget, setDeleteTarget] = useState<ChatAutomation>();
	const [editor, setEditor] = useState<EditorState>();
	// The only copy of a new webhook secret. Never cache or persist it.
	const [webhookSecret, setWebhookSecret] = useState<{
		automationId: string;
		secret: string;
	}>();
	// The editor's opener, or the Rotate secret button during a rotation.
	const secretReturnFocusRef = useRef<HTMLElement | null>(null);

	const automationsQuery = useQuery({
		...chatAutomations(organizationId),
		enabled: Boolean(organizationId),
		// Keeps the server-computed next run current while the page is open.
		refetchInterval: 60_000,
	});
	const chatsQuery = useInfiniteQuery({
		...automationChats(chatsAutomation?.id ?? ""),
		enabled: Boolean(chatsAutomation),
		// Scheduled and webhook runs add chats while the dialog is open. Each
		// tick refetches every loaded page, which stays small in practice.
		refetchInterval: 30_000,
	});
	const updateMutation = useMutation(
		updateChatAutomation(queryClient, organizationId),
	);
	const runMutation = useMutation(
		runChatAutomation(queryClient, organizationId),
	);
	const editMutation = useMutation(
		updateChatAutomation(queryClient, organizationId),
	);
	const deleteMutation = useMutation(
		deleteChatAutomation(queryClient, organizationId),
	);
	const createMutation = useMutation({
		...createChatAutomation(
			queryClient,
			organizationId,
			// Closes the editor in the same render that opens the secret dialog.
			(automationId, secret) => {
				setEditor(undefined);
				setWebhookSecret({ automationId, secret });
			},
		),
		onSuccess: ({ automation }) => {
			toast.success(`Created ${automation.name}.`);
			setEditor(undefined);
		},
	});
	const rotateMutation = useMutation(
		rotateChatAutomationSecret(
			queryClient,
			organizationId,
			(automationId, secret) => setWebhookSecret({ automationId, secret }),
		),
	);

	// Leaving mid-request would drop the one-time secret in the response.
	const leavePrompt = useUnsavedChangesPrompt(
		rotateMutation.isPending ||
			(createMutation.isPending &&
				createMutation.variables?.kind === "webhook"),
	);

	const openEditor = (next: EditorState) => {
		createMutation.reset();
		editMutation.reset();
		rotateMutation.reset();
		secretReturnFocusRef.current =
			document.activeElement instanceof HTMLElement
				? document.activeElement
				: null;
		setEditor(next);
	};

	// Each save or rotate replaces the other's error so alerts do not stack.
	const handleRotateSecret = (
		automation: ChatAutomation,
		rotateButton: HTMLButtonElement | null,
	) => {
		editMutation.reset();
		secretReturnFocusRef.current = rotateButton;
		rotateMutation.mutate(automation.id);
	};

	const handleUpdate = (
		automation: ChatAutomation,
		req: UpdateChatAutomationRequest,
	) => {
		rotateMutation.reset();
		editMutation.mutate(
			{ automationId: automation.id, req },
			{
				onSuccess: (updated) => {
					toast.success(`Saved ${updated.name}.`);
					setEditor(undefined);
				},
			},
		);
	};

	const handleOrganizationChange = (organization: Organization) => {
		setSelectedOrgId(organization.id);
		localStorage.setItem(selectedOrganizationIdStorageKey, organization.id);
		setRunError(undefined);
		setEditor(undefined);
		setDeleteTarget(undefined);
	};

	const openDeleteDialog = (automation: ChatAutomation) => {
		deleteMutation.reset();
		setDeleteTarget(automation);
	};

	const handleDelete = (automation: ChatAutomation) => {
		deleteMutation.mutate(automation.id, {
			onSuccess: () => {
				toast.success(`Deleted ${automation.name}.`);
				setDeleteTarget(undefined);
			},
			onError: (error) => {
				// Someone else deleted it first, so there is nothing to retry.
				if (getErrorStatus(error) === 404) {
					toast.message(`${automation.name} was already deleted.`);
					setDeleteTarget(undefined);
				}
			},
		});
	};

	const handleToggleEnabled = (
		automation: ChatAutomation,
		enabled: boolean,
	) => {
		// mutate's per-call callbacks run only for the latest call, so a later
		// toggle on another row would swallow this row's error toast.
		updateMutation
			.mutateAsync({ automationId: automation.id, req: { enabled } })
			.catch((error) => {
				toast.error(
					getErrorMessage(error, `Could not update ${automation.name}.`),
					{ description: getErrorDetail(error) },
				);
			});
	};

	const handleRunNow = (automation: ChatAutomation) => {
		setRunError(undefined);
		runMutation.mutate(automation.id, {
			onSuccess: () => {
				toast.success(`${automation.name} accepted the run.`);
			},
			onError: (error) => {
				setRunError({ automation, error, organizationId });
			},
		});
	};

	return (
		<AgentAutomationsPageView
			header={<AutomationsPageHeader />}
			currentUserId={user.id}
			organizationSelector={
				showOrganizations && (
					<CompactOrgSelector
						value={selectedOrg ?? null}
						options={organizations}
						onChange={handleOrganizationChange}
						dropdownAlign="end"
					/>
				)
			}
			automations={automationsQuery.data}
			isLoading={automationsQuery.isLoading}
			error={automationsQuery.error}
			updatingAutomationId={
				updateMutation.isPending
					? updateMutation.variables?.automationId
					: undefined
			}
			runningAutomationId={
				runMutation.isPending ? runMutation.variables : undefined
			}
			runError={
				runError?.organizationId === organizationId ? runError : undefined
			}
			onDismissRunError={() => setRunError(undefined)}
			onToggleEnabled={handleToggleEnabled}
			onRunNow={handleRunNow}
			onViewChats={setChatsAutomation}
			onCreateAutomation={() => openEditor({ mode: "create" })}
			onEditAutomation={(automation) =>
				openEditor({ mode: "edit", automation })
			}
			onDeleteAutomation={openDeleteDialog}
			deleteDialog={
				deleteTarget && {
					automation: deleteTarget,
					isDeleting: deleteMutation.isPending,
					error: deleteMutation.error,
					onConfirm: () => handleDelete(deleteTarget),
					onClose: () => setDeleteTarget(undefined),
				}
			}
			editorDialog={
				editor && (
					<AutomationEditorDialog
						organizationId={organizationId}
						automation={editor.mode === "edit" ? editor.automation : undefined}
						currentUserId={user.id}
						origin={webhookOrigin}
						error={
							editor.mode === "edit" ? editMutation.error : createMutation.error
						}
						isSubmitting={
							editor.mode === "edit"
								? editMutation.isPending
								: createMutation.isPending
						}
						onCreate={createMutation.mutate}
						onUpdate={(req) => {
							if (editor.mode === "edit") {
								handleUpdate(editor.automation, req);
							}
						}}
						rotateSecretError={rotateMutation.error}
						isRotatingSecret={rotateMutation.isPending}
						onRotateSecret={(rotateButton) => {
							if (editor.mode === "edit") {
								handleRotateSecret(editor.automation, rotateButton);
							}
						}}
						onClose={() => setEditor(undefined)}
					/>
				)
			}
			webhookSecretDialog={
				<>
					{webhookSecret && (
						<AutomationWebhookSecretDialog
							endpoint={webhookPublishEndpoint(
								webhookOrigin,
								webhookSecret.automationId,
							)}
							secret={webhookSecret.secret}
							returnFocusRef={secretReturnFocusRef}
							onClose={() => setWebhookSecret(undefined)}
						/>
					)}
					{leavePrompt.isOpen && (
						<LeaveBeforeSecretPrompt
							onStay={leavePrompt.onCancel}
							onLeave={leavePrompt.onConfirm}
						/>
					)}
				</>
			}
			chatsDialog={
				chatsAutomation && {
					automation: chatsAutomation,
					chats: chatsQuery.data,
					isLoading: chatsQuery.isLoading,
					error: chatsQuery.error,
					hasNextPage: chatsQuery.hasNextPage,
					isFetchingNextPage: chatsQuery.isFetchingNextPage,
					onLoadMore: () => void chatsQuery.fetchNextPage(),
					onClose: () => setChatsAutomation(undefined),
				}
			}
		/>
	);
};

type LeaveBeforeSecretPromptProps = {
	onStay: () => void;
	onLeave: () => void;
};

const LeaveBeforeSecretPrompt: React.FC<LeaveBeforeSecretPromptProps> = ({
	onStay,
	onLeave,
}) => {
	// The prompt opens without a trigger, so Radix has nowhere to return focus.
	const [opener] = useState(() =>
		document.activeElement instanceof HTMLElement
			? document.activeElement
			: null,
	);
	return (
		<ConfirmDialog
			open
			type="info"
			hideCancel={false}
			cancelText="Stay"
			title="Leave before the secret arrives?"
			description="The webhook secret is shown only once. If you leave now, you must rotate it to get a new one."
			confirmText="Leave"
			onClose={onStay}
			onConfirm={onLeave}
			onCloseAutoFocus={(event) => {
				if (opener?.isConnected) {
					event.preventDefault();
					opener.focus();
				}
			}}
		/>
	);
};

export default AgentAutomationsPage;
