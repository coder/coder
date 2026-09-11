import { FolderInputIcon } from "lucide-react";
import type { FC } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { toast } from "sonner";
import { getErrorMessage } from "#/api/errors";
import { chatProjects } from "#/api/queries/chatProjects";
import { updateChatProject } from "#/api/queries/chats";
import type { Chat } from "#/api/typesGenerated";
import {
	ContextMenuItem,
	ContextMenuSub,
	ContextMenuSubContent,
	ContextMenuSubTrigger,
} from "#/components/ContextMenu/ContextMenu";
import {
	DropdownMenuItem,
	DropdownMenuSub,
	DropdownMenuSubContent,
	DropdownMenuSubTrigger,
} from "#/components/DropdownMenu/DropdownMenu";
import { useDashboard } from "#/modules/dashboard/useDashboard";

type ChatProjectActionsProps = {
	readonly chat: Chat;
	readonly menu: "context" | "dropdown";
};

export const ChatProjectActions: FC<ChatProjectActionsProps> = ({
	chat,
	menu,
}) => {
	const { experiments } = useDashboard();
	const queryClient = useQueryClient();
	const projectsQuery = useQuery({
		...chatProjects(chat.organization_id),
		enabled: experiments.includes("chat-projects"),
	});
	const updateProjectBase = updateChatProject(queryClient);
	const updateProjectMutation = useMutation({
		...updateProjectBase,
		onError: (error, variables, context) => {
			updateProjectBase.onError(error, variables, context);
			toast.error(getErrorMessage(error, "Failed to update project."));
		},
	});

	if (!experiments.includes("chat-projects") || chat.parent_chat_id) {
		return null;
	}

	const selectProject = (projectId: string | null) => {
		updateProjectMutation.mutate({ chatId: chat.id, projectId });
	};
	const Sub = menu === "dropdown" ? DropdownMenuSub : ContextMenuSub;
	const SubTrigger =
		menu === "dropdown" ? DropdownMenuSubTrigger : ContextMenuSubTrigger;
	const SubContent =
		menu === "dropdown" ? DropdownMenuSubContent : ContextMenuSubContent;
	const Item = menu === "dropdown" ? DropdownMenuItem : ContextMenuItem;

	return (
		<Sub>
			<SubTrigger disabled={projectsQuery.isLoading}>
				<FolderInputIcon className="size-3.5" />
				Move to project
			</SubTrigger>
			<SubContent>
				<Item
					disabled={updateProjectMutation.isPending}
					onSelect={() => selectProject(null)}
				>
					No project
				</Item>
				{projectsQuery.data?.map((project) => (
					<Item
						key={project.id}
						disabled={updateProjectMutation.isPending}
						onSelect={() => selectProject(project.id)}
					>
						{project.name}
					</Item>
				))}
			</SubContent>
		</Sub>
	);
};
