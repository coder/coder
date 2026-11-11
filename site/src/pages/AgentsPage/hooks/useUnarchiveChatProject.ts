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
 * Unarchives one project and toasts the outcome. Every caller for the same
 * project shares the pending state, so a project page's menu and banner
 * can't send the request twice. onError runs after the error toast.
 */
export const useUnarchiveChatProject = (
	project: ChatProject,
	onError?: () => void,
) => {
	const queryClient = useQueryClient();
	const mutationKey = unarchiveChatProjectMutationKey(project.id);
	const options = unarchiveChatProject(queryClient);
	const mutation = useMutation({
		...options,
		mutationKey,
		onSuccess: (...args) => {
			options.onSuccess?.(...args);
			toast.success("Project unarchived");
		},
		onError: (error) => {
			toast.error(getErrorMessage(error, "Failed to unarchive project."));
			onError?.();
		},
	});
	const isPending = useIsMutating({ mutationKey }) > 0;
	return {
		isPending,
		unarchive: () => {
			// isPending lags a render behind, so a double click would send a
			// second request that fails because the first one already
			// unarchived the project.
			if (queryClient.isMutating({ mutationKey }) === 0) {
				mutation.mutate(project);
			}
		},
	};
};
