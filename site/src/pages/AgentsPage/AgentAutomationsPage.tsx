import { useRef, useState } from "react";
import {
	useInfiniteQuery,
	useMutation,
	useQuery,
	useQueryClient,
} from "react-query";
import { toast } from "sonner";
import { getErrorDetail, getErrorMessage } from "#/api/errors";
import {
	automationChats,
	chatAutomations,
	createChatAutomation,
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
import { useAutomationsEnabled } from "./components/Automations/AutomationsNavItem";
import { AutomationWebhookSecretDialog } from "./components/Automations/AutomationWebhookSecretDialog";
import { CompactOrgSelector } from "./components/ChatElements/CompactOrgSelector";

const AgentAutomationsPage: React.FC = () => {
	const { organizations, showOrganizations } = useDashboard();
	return useAutomationsEnabled() ? (
		<AutomationsList
			organizations={organizations}
			showOrganizations={showOrganizations}
		/>
	) : (
		<NotFoundPage />
	);
};

type EditorState =
	| { mode: "create" }
	| { mode: "edit"; automation: ChatAutomation };

type AutomationsListProps = {
	organizations: readonly Organization[];
	showOrganizations: boolean;
};

const AutomationsList: React.FC<AutomationsListProps> = ({
	organizations,
	showOrganizations,
}) => {
	const queryClient = useQueryClient();
	const { user } = useAuthenticated();
	const { buildInfo } = useDashboard();
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

	const [runError, setRunError] = useState<AutomationRunError>();
	const [chatsAutomation, setChatsAutomation] = useState<ChatAutomation>();
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
	};

	const handleToggleEnabled = (
		automation: ChatAutomation,
		enabled: boolean,
	) => {
		updateMutation.mutate(
			{ automationId: automation.id, req: { enabled } },
			{
				onError: (error) => {
					toast.error(
						getErrorMessage(error, `Could not update ${automation.name}.`),
						{ description: getErrorDetail(error) },
					);
				},
			},
		);
	};

	const handleRunNow = (automation: ChatAutomation) => {
		setRunError(undefined);
		runMutation.mutate(automation.id, {
			onSuccess: () => {
				toast.success(`${automation.name} accepted the run.`);
			},
			onError: (error) => {
				setRunError({ automation, error });
			},
		});
	};

	return (
		<AgentAutomationsPageView
			header={
				<AgentPageHeader mobileBack={{ to: "/agents", label: "Agents" }} />
			}
			currentUserId={user.id}
			organizationName={selectedOrg?.display_name || selectedOrg?.name}
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
			runError={runError}
			onDismissRunError={() => setRunError(undefined)}
			onToggleEnabled={handleToggleEnabled}
			onRunNow={handleRunNow}
			onViewChats={setChatsAutomation}
			onCreateAutomation={() => openEditor({ mode: "create" })}
			onEditAutomation={(automation) =>
				openEditor({ mode: "edit", automation })
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
					<ConfirmDialog
						open={leavePrompt.isOpen}
						onClose={leavePrompt.onCancel}
						onConfirm={leavePrompt.onConfirm}
						title="Leave before the secret arrives?"
						description="The webhook secret is shown only once. If you leave now, you must rotate it to get a new one."
						confirmText="Leave"
					/>
				</>
			}
			chatsDialog={
				chatsAutomation && {
					automation: chatsAutomation,
					chats: chatsQuery.data?.pages.flat(),
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

export default AgentAutomationsPage;
