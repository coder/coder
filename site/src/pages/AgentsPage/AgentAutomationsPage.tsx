import { useState } from "react";
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
	runChatAutomation,
	updateChatAutomation,
} from "#/api/queries/chatAutomations";
import type {
	ChatAutomation,
	CreateChatAutomationRequest,
	Organization,
	UpdateChatAutomationRequest,
} from "#/api/typesGenerated";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import NotFoundPage from "#/pages/NotFoundPage/NotFoundPage";
import {
	AgentAutomationsPageView,
	type AutomationRunError,
} from "./AgentAutomationsPageView";
import { selectedOrganizationIdStorageKey } from "./components/AgentCreateForm";
import { AgentPageHeader } from "./components/AgentPageHeader";
import { AutomationEditorDialog } from "./components/Automations/AutomationEditorDialog";
import { useAutomationsEnabled } from "./components/Automations/automationsFlag";
import { CompactOrgSelector } from "./components/ChatElements/CompactOrgSelector";

const AgentAutomationsPage: React.FC = () => {
	return useAutomationsEnabled() ? <AutomationsList /> : <NotFoundPage />;
};

type EditorState =
	| { mode: "create" }
	| { mode: "edit"; automation: ChatAutomation };

const AutomationsList: React.FC = () => {
	const { organizations, showOrganizations } = useDashboard();
	const queryClient = useQueryClient();
	const { user } = useAuthenticated();
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
	const createMutation = useMutation(
		createChatAutomation(queryClient, organizationId),
	);
	const editMutation = useMutation(
		updateChatAutomation(queryClient, organizationId),
	);

	const openEditor = (next: EditorState) => {
		createMutation.reset();
		editMutation.reset();
		setEditor(next);
	};

	const handleCreate = (req: CreateChatAutomationRequest) => {
		createMutation.mutate(req, {
			onSuccess: ({ automation }) => {
				toast.success(`Created ${automation.name}.`);
				setEditor(undefined);
			},
		});
	};

	const handleUpdate = (
		automation: ChatAutomation,
		req: UpdateChatAutomationRequest,
	) => {
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
						error={
							editor.mode === "edit" ? editMutation.error : createMutation.error
						}
						isSubmitting={
							editor.mode === "edit"
								? editMutation.isPending
								: createMutation.isPending
						}
						onCreate={handleCreate}
						onUpdate={(req) => {
							if (editor.mode === "edit") {
								handleUpdate(editor.automation, req);
							}
						}}
						onClose={() => setEditor(undefined)}
					/>
				)
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
