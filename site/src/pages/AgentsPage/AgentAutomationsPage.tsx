import { useState } from "react";
import {
	useInfiniteQuery,
	useMutation,
	useQuery,
	useQueryClient,
} from "react-query";
import { useLocation, useNavigate, useSearchParams } from "react-router";
import { toast } from "sonner";
import { getErrorDetail, getErrorMessage, getErrorStatus } from "#/api/errors";
import { checkAuthorization } from "#/api/queries/authCheck";
import {
	automationChats,
	chatAutomations,
	deleteChatAutomation,
	fetchSkippedRun,
	runChatAutomation,
	updateChatAutomation,
} from "#/api/queries/chatAutomations";
import type { ChatAutomation, Organization } from "#/api/typesGenerated";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import NotFoundPage from "#/pages/NotFoundPage/NotFoundPage";
import {
	AgentAutomationsPageView,
	type AutomationRunError,
} from "./AgentAutomationsPageView";
import { selectedOrganizationIdStorageKey } from "./components/AgentCreateForm";
import { AgentPageHeader } from "./components/AgentPageHeader";
import { skippedRunMessage } from "./components/Automations/AutomationLastRun";
import type { AutomationOwnerView } from "./components/Automations/AutomationOwnerFilter";
import {
	automationPath,
	newAutomationPath,
	useAutomationsEnabled,
} from "./components/Automations/automationsFlag";
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

type AutomationsPageHeaderProps = {
	/** Top bar content on wide screens, such as a breadcrumb. */
	children?: React.ReactNode;
	/** The mobile back link; defaults to Agents. */
	backTo?: { pathname: string; label: string };
};

/** Keeps the sidebar's search filters on the mobile back link. */
export const AutomationsPageHeader: React.FC<AutomationsPageHeaderProps> = ({
	children,
	backTo = { pathname: "/agents", label: "Agents" },
}) => {
	const location = useLocation();
	return (
		<AgentPageHeader
			mobileBack={{
				to: {
					pathname: backTo.pathname,
					search: normalizeLocationSearch(location.search),
				},
				label: backTo.label,
			}}
		>
			{children}
		</AgentPageHeader>
	);
};

/** The organization the Agents organization picker selected last. */
export const useSelectedAutomationsOrganization = () => {
	const { organizations } = useDashboard();
	const [selectedOrgId, setSelectedOrgId] = useState(() =>
		localStorage.getItem(selectedOrganizationIdStorageKey),
	);
	const selectedOrg =
		organizations.find((org) => org.id === selectedOrgId) ??
		organizations.find((org) => org.is_default) ??
		organizations[0];
	const selectOrganization = (organization: Organization) => {
		setSelectedOrgId(organization.id);
		localStorage.setItem(selectedOrganizationIdStorageKey, organization.id);
	};
	return { selectedOrg, selectOrganization };
};

const AutomationsList: React.FC = () => {
	const { organizations, showOrganizations } = useDashboard();
	const queryClient = useQueryClient();
	const { user } = useAuthenticated();
	const navigate = useNavigate();
	const { selectedOrg, selectOrganization } =
		useSelectedAutomationsOrganization();
	const organizationId = selectedOrg?.id ?? "";
	// ?owner=all keeps the All view across reloads and Back.
	const [searchParams, setSearchParams] = useSearchParams();
	const wantsAllOwners = searchParams.get("owner") === "all";

	// Holds the organization of the run, so a failure that arrives after the
	// viewer switched organizations does not show on the new one.
	const [runError, setRunError] = useState<
		AutomationRunError & { organizationId: string }
	>();
	const [chatsAutomation, setChatsAutomation] = useState<ChatAutomation>();
	const [deleteTarget, setDeleteTarget] = useState<ChatAutomation>();

	// An object with no owner asks about every automation in the
	// organization, which only admins can read or delete.
	const permissionsQuery = useQuery({
		...checkAuthorization({
			checks: {
				canReadAny: {
					object: {
						resource_type: "chat_automation",
						organization_id: organizationId,
					},
					action: "read",
				},
				canDeleteAny: {
					object: {
						resource_type: "chat_automation",
						organization_id: organizationId,
					},
					action: "delete",
				},
			},
		}),
		enabled: Boolean(organizationId),
	});
	const canReadAny = Boolean(permissionsQuery.data?.canReadAny);
	// Waits for the permission check before asking for every owner, so a
	// member with ?owner=all in the URL gets their own list once.
	const isCheckingOwnerView =
		wantsAllOwners && Boolean(organizationId) && !permissionsQuery.isFetched;
	const ownerView: AutomationOwnerView =
		wantsAllOwners && canReadAny ? "all" : "mine";
	const automationsQuery = useQuery({
		...chatAutomations(
			organizationId,
			ownerView === "all" ? undefined : { owner: "me" },
		),
		enabled: Boolean(organizationId) && !isCheckingOwnerView,
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
	const runMutation = useMutation(
		runChatAutomation(queryClient, organizationId),
	);
	const updateMutation = useMutation(
		updateChatAutomation(queryClient, organizationId),
	);
	const deleteMutation = useMutation(
		deleteChatAutomation(queryClient, organizationId),
	);

	const handleOwnerViewChange = (view: AutomationOwnerView) => {
		const next = new URLSearchParams(searchParams);
		if (view === "all") {
			next.set("owner", "all");
		} else {
			next.delete("owner");
		}
		setSearchParams(next, { replace: true });
	};

	const handleOrganizationChange = (organization: Organization) => {
		selectOrganization(organization);
		setRunError(undefined);
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
			onSuccess: ({ chat_id }) => {
				toast.success(`${automation.name} accepted the run.`, {
					action: {
						label: "Open chat",
						onClick: () => navigate(`/agents/${chat_id}`),
					},
				});
			},
			onError: (error) => {
				void fetchSkippedRun(queryClient, organizationId, automation)
					.catch(() => undefined)
					.then((skipped) => {
						if (skipped) {
							toast.message(skippedRunMessage(automation.name, skipped));
							return;
						}
						setRunError({ automation, error, organizationId });
					});
			},
		});
	};

	return (
		<AgentAutomationsPageView
			header={<AutomationsPageHeader />}
			currentUserId={user.id}
			canDeleteAnyAutomation={Boolean(permissionsQuery.data?.canDeleteAny)}
			ownerView={ownerView}
			onOwnerViewChange={canReadAny ? handleOwnerViewChange : undefined}
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
			isLoading={automationsQuery.isLoading || isCheckingOwnerView}
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
			onCreateAutomation={() => navigate(newAutomationPath)}
			onEditAutomation={(automation) => navigate(automationPath(automation.id))}
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

export default AgentAutomationsPage;
