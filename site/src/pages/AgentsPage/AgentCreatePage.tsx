import { type FC, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { Navigate, useLocation, useNavigate, useParams } from "react-router";
import { toast } from "sonner";
import { getErrorMessage, isApiError } from "#/api/errors";
import { chatProject } from "#/api/queries/chatProjects";
import { createChat } from "#/api/queries/chats";
import { workspaces } from "#/api/queries/workspaces";
import type * as TypesGen from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { useWebpushNotifications } from "#/contexts/useWebpushNotifications";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { useAIGatewayEnabled } from "#/hooks/useEmbeddedMetadata";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import {
	AgentCreateForm,
	type CreateChatOptions,
} from "./components/AgentCreateForm";
import { AgentPageHeader } from "./components/AgentPageHeader";
import { ChimeButton } from "./components/ChimeButton";
import {
	ProjectComposerFooter,
	ProjectComposerHeader,
} from "./components/ProjectComposerFrame";
import { WebPushButton } from "./components/WebPushButton";
import { getChimeEnabled, setChimeEnabled } from "./utils/chime";
import { buildAgentChatPath } from "./utils/navigation";

const lastModelConfigIDStorageKey = "agents.last-model-config-id";

/**
 * New-chat page. Serves both `/agents` and `/agents/projects/:projectId`; in
 * the latter case the composer is framed by the project and the created chat
 * joins it.
 */
const AgentCreatePage: FC = () => {
	const queryClient = useQueryClient();
	const location = useLocation();
	const navigate = useNavigate();
	const { projectId = "" } = useParams<{ projectId?: string }>();
	const { permissions } = useAuthenticated();
	const { experiments } = useDashboard();
	const chatProjectsEnabled = experiments.includes("chat-projects");
	const projectQuery = useQuery({
		...chatProject(projectId),
		enabled: chatProjectsEnabled && Boolean(projectId),
	});
	const selectedProject = projectQuery.data;
	const isProjectMissing =
		isApiError(projectQuery.error) &&
		projectQuery.error.response.status === 404;
	const projectLookupError =
		projectId && chatProjectsEnabled && projectQuery.error && !isProjectMissing
			? projectQuery.error
			: undefined;
	const isProjectLookupPending =
		Boolean(projectId) && chatProjectsEnabled && projectQuery.isLoading;
	const aiGatewayDisabled = !useAIGatewayEnabled();
	const workspacesQuery = useQuery(workspaces({ q: "owner:me", limit: 0 }));
	const createMutation = useMutation(createChat(queryClient));
	const webPush = useWebpushNotifications();
	const [chimeEnabled, setChimeEnabledState] = useState(getChimeEnabled);

	if (projectId && (!chatProjectsEnabled || isProjectMissing)) {
		return <Navigate to="/agents" replace />;
	}

	const handleCreateChat = async ({
		message,
		fileIDs,
		workspaceId,
		model,
		reasoningEffort,
		mcpServerIds,
		organizationId,
		planMode,
	}: CreateChatOptions) => {
		if (isProjectLookupPending || projectLookupError) {
			return;
		}
		const content: TypesGen.ChatInputPart[] = [];
		if (message.trim()) {
			content.push({ type: "text", text: message });
		}
		if (fileIDs) {
			for (const fileID of fileIDs) {
				content.push({ type: "file", file_id: fileID });
			}
		}
		const createRequest: TypesGen.CreateChatRequest = {
			organization_id: selectedProject?.organization_id ?? organizationId,
			content,
			workspace_id: workspaceId,
			mcp_server_ids:
				mcpServerIds && mcpServerIds.length > 0 ? mcpServerIds : undefined,
			plan_mode: planMode === "plan" ? "plan" : undefined,
			client_type: "ui",
			...(model ? { model_config_id: model } : {}),
			...(reasoningEffort ? { reasoning_effort: reasoningEffort } : {}),
			...(selectedProject ? { project_id: selectedProject.id } : {}),
		};
		const createdChat = await createMutation.mutateAsync(createRequest);

		if (model) {
			localStorage.setItem(lastModelConfigIDStorageKey, model);
		}
		navigate({
			pathname: buildAgentChatPath({ chatId: createdChat.id }),
			search: location.search,
		});
	};

	const handleChimeToggle = () => {
		const next = !chimeEnabled;
		setChimeEnabledState(next);
		setChimeEnabled(next);
	};

	const handleNotificationToggle = async () => {
		try {
			if (webPush.subscribed) {
				await webPush.unsubscribe();
			} else {
				await webPush.subscribe();
			}
		} catch (error) {
			const action = webPush.subscribed ? "disable" : "enable";
			toast.error(getErrorMessage(error, `Failed to ${action} notifications.`));
		}
	};

	return (
		<>
			<AgentPageHeader
				chimeEnabled={chimeEnabled}
				onToggleChime={handleChimeToggle}
				webPush={webPush}
				onToggleNotifications={handleNotificationToggle}
			>
				<ChimeButton enabled={chimeEnabled} onToggle={handleChimeToggle} />
				<WebPushButton webPush={webPush} onToggle={handleNotificationToggle} />
			</AgentPageHeader>
			{projectLookupError && (
				<ErrorAlert
					error={projectLookupError}
					className="mx-auto mt-4 w-full max-w-3xl"
				/>
			)}
			<AgentCreateForm
				header={
					selectedProject && <ProjectComposerHeader project={selectedProject} />
				}
				footer={
					selectedProject && <ProjectComposerFooter project={selectedProject} />
				}
				onCreateChat={handleCreateChat}
				isCreating={
					createMutation.isPending ||
					isProjectLookupPending ||
					Boolean(projectLookupError)
				}
				createError={createMutation.error}
				canCreateChat={permissions.createChat}
				canConfigureAgentSetup={permissions.editDeploymentConfig}
				aiGatewayDisabled={aiGatewayDisabled}
				workspaceCount={workspacesQuery.data?.count}
				workspaceOptions={workspacesQuery.data?.workspaces ?? []}
				workspacesError={workspacesQuery.error}
				isWorkspacesLoading={workspacesQuery.isLoading}
			/>
		</>
	);
};

export default AgentCreatePage;
