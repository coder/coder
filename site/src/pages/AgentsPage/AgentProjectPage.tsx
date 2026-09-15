import { type FC, useState } from "react";
import {
	useInfiniteQuery,
	useMutation,
	useQuery,
	useQueryClient,
} from "react-query";
import { Navigate, useParams } from "react-router";
import {
	chatProject,
	chatProjectPermissions,
	updateChatProject,
} from "#/api/queries/chatProjects";
import { infiniteChats } from "#/api/queries/chats";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { chatProjectPermissionsFor } from "#/modules/permissions/chatProjects";
import { AgentProjectPageView } from "./AgentProjectPageView";
import { ChatProjectDialog } from "./components/ChatsSidebar/dialogs/ChatProjectDialog";

const AgentProjectPage: FC = () => {
	const { projectId = "" } = useParams<{ projectId: string }>();
	const queryClient = useQueryClient();
	const { experiments } = useDashboard();
	const [isEditing, setIsEditing] = useState(false);
	const projectQuery = useQuery({
		...chatProject(projectId),
		enabled: experiments.includes("chat-projects") && Boolean(projectId),
	});
	const updateProjectMutation = useMutation(updateChatProject(queryClient));
	const project = projectQuery.data;
	const permissionsQuery = useQuery({
		...chatProjectPermissions(project ? [project] : []),
		enabled: Boolean(project),
	});
	const canEdit = project
		? chatProjectPermissionsFor(project, permissionsQuery.data).canUpdate
		: false;
	const chatsQuery = useInfiniteQuery({
		...infiniteChats({ projectId }),
		enabled: experiments.includes("chat-projects") && Boolean(projectId),
	});
	const chats = chatsQuery.data?.pages.flat() ?? [];

	if (!experiments.includes("chat-projects")) {
		return <Navigate to="/agents" replace />;
	}

	return (
		<>
			<AgentProjectPageView
				project={project}
				chats={chats}
				isLoading={projectQuery.isLoading || chatsQuery.isLoading}
				error={projectQuery.error ?? chatsQuery.error}
				onEdit={canEdit ? () => setIsEditing(true) : undefined}
				newChatPath={`/agents?project=${encodeURIComponent(projectId)}`}
			/>
			<ChatProjectDialog
				key={project?.id ?? (isEditing ? "new" : "closed")}
				organizationId={project?.organization_id ?? ""}
				project={project}
				open={isEditing}
				onOpenChange={setIsEditing}
				onSubmit={async (request) => {
					await updateProjectMutation.mutateAsync({
						projectId,
						request,
					});
				}}
			/>
		</>
	);
};

export default AgentProjectPage;
