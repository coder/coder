import { XIcon } from "lucide-react";
import { type FC, useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { useLocation, useNavigate, useSearchParams } from "react-router";
import { toast } from "sonner";
import { getErrorMessage } from "#/api/errors";
import { chatProjects } from "#/api/queries/chatProjects";
import { createChat } from "#/api/queries/chats";
import { workspaces } from "#/api/queries/workspaces";
import type * as TypesGen from "#/api/typesGenerated";
import { Badge } from "#/components/Badge/Badge";
import { Button } from "#/components/Button/Button";
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
import { WebPushButton } from "./components/WebPushButton";
import { getChimeEnabled, setChimeEnabled } from "./utils/chime";
import { buildAgentChatPath } from "./utils/navigation";

const lastModelConfigIDStorageKey = "agents.last-model-config-id";

const AgentCreatePage: FC = () => {
	const queryClient = useQueryClient();
	const location = useLocation();
	const navigate = useNavigate();
	const [searchParams, setSearchParams] = useSearchParams();
	const { permissions } = useAuthenticated();
	const { experiments, organizations } = useDashboard();
	const defaultOrganizationId =
		organizations.find((organization) => organization.is_default)?.id ?? "";
	const projectId = searchParams.get("project") ?? "";
	const projectQuery = useQuery({
		...chatProjects(defaultOrganizationId),
		enabled: experiments.includes("chat-projects") && Boolean(projectId),
	});
	const selectedProject = projectQuery.data?.find(
		(project) => project.id === projectId,
	);
	const aiGatewayDisabled = !useAIGatewayEnabled();
	const workspacesQuery = useQuery(workspaces({ q: "owner:me", limit: 0 }));
	const createMutation = useMutation(createChat(queryClient));
	const webPush = useWebpushNotifications();
	const [chimeEnabled, setChimeEnabledState] = useState(getChimeEnabled);

	useEffect(() => {
		if (
			!projectId ||
			!experiments.includes("chat-projects") ||
			!projectQuery.isSuccess ||
			selectedProject
		) {
			return;
		}
		const nextSearchParams = new URLSearchParams(searchParams);
		nextSearchParams.delete("project");
		setSearchParams(nextSearchParams, { replace: true });
	}, [
		experiments,
		projectId,
		projectQuery.isSuccess,
		searchParams,
		selectedProject,
		setSearchParams,
	]);

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
			...(selectedProject?.organization_id === organizationId
				? { project_id: selectedProject.id }
				: {}),
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
			{selectedProject && (
				<div className="mx-auto w-full max-w-3xl px-4 pt-4">
					<Badge size="sm" className="w-fit">
						Project: {selectedProject.name}
						<Button
							variant="subtle"
							size="icon"
							className="-my-1 size-5 min-w-0"
							aria-label="Remove project"
							onClick={() => {
								const nextSearchParams = new URLSearchParams(searchParams);
								nextSearchParams.delete("project");
								setSearchParams(nextSearchParams);
							}}
						>
							<XIcon className="size-3" />
						</Button>
					</Badge>
				</div>
			)}
			<AgentCreateForm
				onCreateChat={handleCreateChat}
				isCreating={createMutation.isPending}
				createError={createMutation.error}
				canCreateChat={permissions.createChat}
				canConfigureAgentSetup={permissions.editDeploymentConfig}
				aiGatewayDisabled={aiGatewayDisabled}
				workspaceCount={workspacesQuery.data?.count}
				workspaceOptions={workspacesQuery.data?.workspaces ?? []}
				workspacesError={workspacesQuery.error}
				isWorkspacesLoading={workspacesQuery.isLoading}
			/>{" "}
		</>
	);
};

export default AgentCreatePage;
