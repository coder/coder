import { useIsMutating, useMutation, useQueryClient } from "react-query";
import { getErrorMessage } from "#/api/errors";
import {
	chatProjectsKey,
	unarchiveChatProject,
} from "#/api/queries/chatProjects";
import type { ChatProject } from "#/api/typesGenerated";
import { toast } from "#/components/Toaster/toast";

/**
 * Unarchives one project and toasts the outcome. Every caller for the same
 * project shares the pending state, so a project page's menu and banner
 * can't send the request twice. onSuccess runs after the success toast,
 * even when the caller has unmounted.
 */
export const useUnarchiveChatProject = (
	project: ChatProject,
	onSuccess?: () => void,
) => {
	const queryClient = useQueryClient();
	const mutationKey = [...chatProjectsKey, "unarchive", project.id];
	const options = unarchiveChatProject(queryClient);
	const mutation = useMutation({
		...options,
		mutationKey,
		onSuccess: (...args) => {
			options.onSuccess?.(...args);
			toast.success("Project unarchived");
			onSuccess?.();
		},
		onError: (error) => {
			toast.error(getErrorMessage(error, "Failed to unarchive project."));
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
