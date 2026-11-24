import { useQueryClient } from "react-query";
import {
	invalidateChatEntity,
	invalidateChatsByWorkspace,
} from "#/api/queries/chats";
import { invalidateWorkspaceMutationQueries } from "#/api/queries/workspaces";

type UseChatToolInvalidationsOptions = {
	chatID: string;
	organizationName: string;
	username: string;
};

const CHAT_WORKSPACE_BINDING_TOOL_NAMES = new Set(["create_workspace"]);
const WORKSPACE_MUTATION_TOOL_NAMES = new Set([
	"create_workspace",
	"start_workspace",
	"stop_workspace",
]);

/**
 * Returns a handler for completed chat tool results that invalidates derived
 * UI data for the server state those tools may have changed.
 */
export function useChatToolInvalidations({
	chatID,
	organizationName,
	username,
}: UseChatToolInvalidationsOptions): (toolNames: readonly string[]) => void {
	const queryClient = useQueryClient();

	return (toolNames) => {
		if (toolNames.some((name) => CHAT_WORKSPACE_BINDING_TOOL_NAMES.has(name))) {
			void invalidateChatEntity(queryClient, chatID);
			void invalidateChatsByWorkspace(queryClient);
		}

		if (toolNames.some((name) => WORKSPACE_MUTATION_TOOL_NAMES.has(name))) {
			void invalidateWorkspaceMutationQueries(queryClient, {
				organizationName,
				username,
			});
		}
	};
}
