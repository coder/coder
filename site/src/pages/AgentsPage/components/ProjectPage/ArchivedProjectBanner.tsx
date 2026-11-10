import { useMutation, useQueryClient } from "react-query";
import { getErrorMessage } from "#/api/errors";
import { unarchiveChatProject } from "#/api/queries/chatProjects";
import type { ChatProject } from "#/api/typesGenerated";
import { Alert, AlertDescription, AlertTitle } from "#/components/Alert/Alert";
import { Button } from "#/components/Button/Button";
import { Spinner } from "#/components/Spinner/Spinner";
import { toast } from "#/components/Toaster/toast";

type ArchivedProjectBannerProps = {
	readonly project: ChatProject;
};

/** Replaces an archived project's composer, with a way to unarchive it. */
export const ArchivedProjectBanner: React.FC<ArchivedProjectBannerProps> = ({
	project,
}) => {
	const queryClient = useQueryClient();
	const unarchiveMutation = useMutation({
		...unarchiveChatProject(queryClient),
		onError: (error) => {
			toast.error(getErrorMessage(error, "Failed to unarchive project."));
		},
	});

	return (
		<Alert
			severity="info"
			prominent
			actions={
				project.permissions.archive && (
					<Button
						size="sm"
						variant="outline"
						disabled={unarchiveMutation.isPending}
						onClick={() => unarchiveMutation.mutate(project)}
					>
						<Spinner loading={unarchiveMutation.isPending} />
						Unarchive
					</Button>
				)
			}
		>
			<AlertTitle>This project is archived.</AlertTitle>
			<AlertDescription>
				Its chats are read-only, and no new chats can start in it.
			</AlertDescription>
		</Alert>
	);
};
