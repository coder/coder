import { PencilIcon } from "lucide-react";
import { type FC, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { Navigate, useLocation, useNavigate, useParams } from "react-router";
import { toast } from "sonner";
import { getErrorMessage, isApiError } from "#/api/errors";
import { chatProject, updateChatProject } from "#/api/queries/chatProjects";
import { createChat } from "#/api/queries/chats";
import { workspaces } from "#/api/queries/workspaces";
import type * as TypesGen from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Button } from "#/components/Button/Button";
import { Loader } from "#/components/Loader/Loader";
import { useWebpushNotifications } from "#/contexts/useWebpushNotifications";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { useAIGatewayEnabled } from "#/hooks/useEmbeddedMetadata";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import {
	AgentCreateForm,
	type CreateChatOptions,
} from "./components/AgentCreateForm";
import { AgentPageHeader } from "./components/AgentPageHeader";
import { ChatProjectDialog } from "./components/ChatsSidebar/dialogs/ChatProjectDialog";
import { ChimeButton } from "./components/ChimeButton";
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
	const { projectId } = useParams<{ projectId?: string }>();
	const { permissions } = useAuthenticated();
	const { experiments } = useDashboard();
	const chatProjectsEnabled = experiments.includes("chat-projects");
	const projectQuery = useQuery({
		...chatProject(projectId),
		enabled: chatProjectsEnabled && projectId !== undefined,
	});
	const selectedProject = projectQuery.data;
	const isProjectMissing =
		isApiError(projectQuery.error) &&
		projectQuery.error.response.status === 404;
	// A cached project stays usable when a background refetch fails; only a
	// lookup with nothing to show blocks the composer.
	const projectLookupError =
		projectId !== undefined &&
		chatProjectsEnabled &&
		!selectedProject &&
		projectQuery.error &&
		!isProjectMissing
			? projectQuery.error
			: undefined;
	const aiGatewayDisabled = !useAIGatewayEnabled();
	const workspacesQuery = useQuery(workspaces({ q: "owner:me", limit: 0 }));
	const createMutation = useMutation(createChat(queryClient));
	// The mutation outlives project navigation, so only show its error under
	// the project it was attempted for.
	const attemptedProjectId = createMutation.variables?.project_id;
	const selectedProjectId = selectedProject?.id;
	const createError =
		attemptedProjectId === selectedProjectId ? createMutation.error : undefined;
	const webPush = useWebpushNotifications();
	const [chimeEnabled, setChimeEnabledState] = useState(getChimeEnabled);

	if (projectId !== undefined && (!chatProjectsEnabled || isProjectMissing)) {
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
			organization_id: organizationId,
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
			{projectLookupError ? (
				<ErrorAlert
					error={projectLookupError}
					className="mx-auto mt-4 w-full max-w-3xl"
					actions={
						<Button
							size="sm"
							variant="outline"
							onClick={() => void projectQuery.refetch()}
						>
							Retry
						</Button>
					}
				/>
			) : projectId !== undefined && chatProjectsEnabled && !selectedProject ? (
				// The form must not mount until its organization is known because its
				// attachments and remembered choices are organization-scoped.
				<Loader label="Loading project" />
			) : (
				<AgentCreateForm
					lockedOrganizationId={selectedProject?.organization_id}
					header={
						selectedProject && (
							<div className="mb-4 min-w-0 text-center">
								<h1 className="m-0 break-words text-2xl font-semibold text-content-primary [overflow-wrap:anywhere]">
									{selectedProject.name}
								</h1>
								{selectedProject.description && (
									<p className="mx-auto mb-0 mt-2 max-w-xl break-words text-sm text-content-secondary [overflow-wrap:anywhere]">
										{selectedProject.description}
									</p>
								)}
							</div>
						)
					}
					footer={
						selectedProject && (
							<ProjectComposerFooter
								key={selectedProject.id}
								project={selectedProject}
							/>
						)
					}
					onCreateChat={handleCreateChat}
					isCreating={createMutation.isPending}
					createError={createError}
					canCreateChat={permissions.createChat}
					canConfigureAgentSetup={permissions.editDeploymentConfig}
					aiGatewayDisabled={aiGatewayDisabled}
					workspaceCount={workspacesQuery.data?.count}
					workspaceOptions={workspacesQuery.data?.workspaces ?? []}
					workspacesError={workspacesQuery.error}
					isWorkspacesLoading={workspacesQuery.isLoading}
				/>
			)}
		</>
	);
};

type ProjectComposerFooterProps = {
	readonly project: TypesGen.ChatProject;
};

const ProjectComposerFooter: FC<ProjectComposerFooterProps> = ({ project }) => {
	const queryClient = useQueryClient();
	const [isEditing, setIsEditing] = useState(false);
	const editButtonRef = useRef<HTMLButtonElement>(null);
	const updateProjectMutation = useMutation(updateChatProject(queryClient));
	const closeDialog = () => {
		setIsEditing(false);
		requestAnimationFrame(() => editButtonRef.current?.focus());
	};

	return (
		<div className="flex justify-center pt-2">
			<Button
				ref={editButtonRef}
				variant="subtle"
				size="sm"
				className="text-content-secondary"
				onClick={() => {
					updateProjectMutation.reset();
					setIsEditing(true);
				}}
			>
				<PencilIcon />
				Edit project
			</Button>
			<ChatProjectDialog
				project={project}
				open={isEditing}
				onOpenChange={(open) => {
					if (!open) closeDialog();
				}}
				isSubmitting={updateProjectMutation.isPending}
				error={updateProjectMutation.error}
				onSubmit={(request) => {
					updateProjectMutation.mutate(
						{ projectId: project.id, request },
						{ onSuccess: closeDialog },
					);
				}}
			/>
		</div>
	);
};

export default AgentCreatePage;
