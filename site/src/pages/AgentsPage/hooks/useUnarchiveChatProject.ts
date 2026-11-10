import { useIsMutating, useMutation, useQueryClient } from "react-query";
import { getErrorMessage } from "#/api/errors";
import {
	chatProjectsKey,
	unarchiveChatProject,
} from "#/api/queries/chatProjects";
import type { ChatProject } from "#/api/typesGenerated";
import { toast } from "#/components/Toaster/toast";

const unarchiveChatProjectMutationKey = (projectId: string) =>
	[...chatProjectsKey, "unarchive", projectId] as const;

/**
 * Unarchives one project and toasts a failure. Every caller for the same
 * project shares the pending state, so a project page's menu and banner
 * can't send the request twice.
 */
export const useUnarchiveChatProject = (project: ChatProject) => {
	const queryClient = useQueryClient();
	const mutationKey = unarchiveChatProjectMutationKey(project.id);
	const mutation = useMutation({
		...unarchiveChatProject(queryClient),
		mutationKey,
		onError: (error) => {
			toast.error(getErrorMessage(error, "Failed to unarchive project."));
		},
	});
	const isPending = useIsMutating({ mutationKey }) > 0;
	return {
		isPending,
		unarchive: () => {
			if (!isPending) {
				mutation.mutate(project);
			}
		},
	};
};
