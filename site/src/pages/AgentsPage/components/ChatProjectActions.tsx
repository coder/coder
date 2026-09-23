import { FolderInputIcon } from "lucide-react";
import type { FC } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { toast } from "sonner";
import { getErrorMessage } from "#/api/errors";
import { chatProjects } from "#/api/queries/chatProjects";
import { moveChatToProject } from "#/api/queries/chats";
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
import { ChatProjectIcon } from "./ChatProjectIcon";

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
	const moveChatBase = moveChatToProject(queryClient);
	const moveChatMutation = useMutation({
		...moveChatBase,
		onError: (error, variables, context) => {
			moveChatBase.onError(error, variables, context);
			toast.error(getErrorMessage(error, "Failed to move chat."));
		},
	});

	if (!experiments.includes("chat-projects") || chat.parent_chat_id) {
		return null;
	}

	const selectProject = (projectId: string | null) => {
		moveChatMutation.mutate({ chatId: chat.id, projectId });
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
				<FolderInputIcon />
				Move to project
			</SubTrigger>
			<SubContent>
				<Item
					disabled={moveChatMutation.isPending}
					onSelect={() => selectProject(null)}
				>
					No project
				</Item>
				{projectsQuery.data === undefined && projectsQuery.error ? (
					<>
						<Item disabled className="text-content-destructive">
							{getErrorMessage(projectsQuery.error, "Failed to load projects.")}
						</Item>
						<Item
							onSelect={(event) => {
								event.preventDefault();
								void projectsQuery.refetch();
							}}
						>
							Retry
						</Item>
					</>
				) : (
					(projectsQuery.data ?? [])
						.filter((project) => project.owner_id === chat.owner_id)
						.map((project) => (
							<Item
								key={project.id}
								disabled={moveChatMutation.isPending}
								onSelect={() => selectProject(project.id)}
							>
								<ChatProjectIcon project={project} className="size-4" />
								{project.name}
							</Item>
						))
				)}
			</SubContent>
		</Sub>
	);
};
