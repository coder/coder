import type { ChatProject } from "#/api/typesGenerated";
import { Alert, AlertDescription, AlertTitle } from "#/components/Alert/Alert";
import { Button } from "#/components/Button/Button";
import { Spinner } from "#/components/Spinner/Spinner";
import { useUnarchiveChatProject } from "../../hooks/useUnarchiveChatProject";

type ArchivedProjectBannerProps = {
	readonly project: ChatProject;
};

/**
 * Says the project is archived and, when the user may, offers to unarchive it.
 */
export const ArchivedProjectBanner: React.FC<ArchivedProjectBannerProps> = ({
	project,
}) => {
	const { unarchive, isPending } = useUnarchiveChatProject(project);

	return (
		<Alert
			severity="info"
			prominent
			actions={
				project.permissions.archive && (
					<Button
						size="sm"
						variant="outline"
						disabled={isPending}
						onClick={unarchive}
					>
						<Spinner loading={isPending} />
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
