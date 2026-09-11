import { type FC, useState } from "react";
import {
	useInfiniteQuery,
	useMutation,
	useQuery,
	useQueryClient,
} from "react-query";
import { Navigate, useParams } from "react-router";
import { chatProject, updateChatProject } from "#/api/queries/chatProjects";
import { infiniteChats } from "#/api/queries/chats";
import { useDashboard } from "#/modules/dashboard/useDashboard";
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
				project={projectQuery.data}
				chats={chats}
				isLoading={projectQuery.isLoading || chatsQuery.isLoading}
				error={projectQuery.error ?? chatsQuery.error}
				onEdit={() => setIsEditing(true)}
				newChatPath={`/agents?project=${encodeURIComponent(projectId)}`}
			/>
			<ChatProjectDialog
				organizationId={projectQuery.data?.organization_id ?? ""}
				project={projectQuery.data}
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
