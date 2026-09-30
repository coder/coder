import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { toast } from "sonner";
import { getErrorDetail, getErrorMessage } from "#/api/errors";
import {
	automationChats,
	chatAutomations,
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
import { CompactOrgSelector } from "./components/ChatElements/CompactOrgSelector";

const AgentAutomationsPage: React.FC = () => {
	const { experiments, organizations, showOrganizations } = useDashboard();
	if (!experiments.includes("chat-automations")) {
		return <NotFoundPage />;
	}
	return (
		<AutomationsList
			organizations={organizations}
			showOrganizations={showOrganizations}
		/>
	);
};

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

	const automationsQuery = useQuery({
		...chatAutomations(organizationId),
		enabled: Boolean(organizationId),
	});
	const chatsQuery = useQuery({
		...automationChats(chatsAutomation?.id ?? ""),
		enabled: Boolean(chatsAutomation),
	});
	const updateMutation = useMutation(
		updateChatAutomation(queryClient, organizationId),
	);
	const runMutation = useMutation(
		runChatAutomation(queryClient, organizationId),
	);

	const handleOrganizationChange = (organization: Organization) => {
		setSelectedOrgId(organization.id);
		localStorage.setItem(selectedOrganizationIdStorageKey, organization.id);
		setRunError(undefined);
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
			chatsDialog={
				chatsAutomation && {
					automation: chatsAutomation,
					chats: chatsQuery.data,
					isLoading: chatsQuery.isLoading,
					error: chatsQuery.error,
					onClose: () => setChatsAutomation(undefined),
				}
			}
		/>
	);
};

export default AgentAutomationsPage;
