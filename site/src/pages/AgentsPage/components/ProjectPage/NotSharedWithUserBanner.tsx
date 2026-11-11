import { Alert, AlertDescription, AlertTitle } from "#/components/Alert/Alert";

/**
 * Replaces the composer on a project the user can manage but that isn't
 * shared with them, since the server refuses chats there.
 */
export const NotSharedWithUserBanner: React.FC = () => (
	<Alert severity="info" prominent>
		<AlertTitle>This project isn't shared with you.</AlertTitle>
		<AlertDescription>
			Only its owner and the people it is shared with can start chats in it.
		</AlertDescription>
	</Alert>
);
