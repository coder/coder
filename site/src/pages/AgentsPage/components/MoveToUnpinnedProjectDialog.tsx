import { createContext, useContext, useState } from "react";
import { useMutation, useQueryClient } from "react-query";
import { useLocation, useNavigate } from "react-router";
import { getErrorMessage } from "#/api/errors";
import { pinChatProject } from "#/api/queries/chatProjects";
import type { ChatProject } from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from "#/components/Dialog/Dialog";
import { Spinner } from "#/components/Spinner/Spinner";
import { toast } from "#/components/Toaster/toast";
import { buildAgentProjectPath } from "../utils/navigation";

type ShowMoveToUnpinnedProjectDialog = (project: ChatProject) => void;

const MoveToUnpinnedProjectContext = createContext<
	ShowMoveToUnpinnedProjectDialog | undefined
>(undefined);

/**
 * Opens the dialog that explains a chat moved into an unpinned project left
 * the sidebar. Undefined outside the provider, where no dialog opens.
 */
export const useShowMoveToUnpinnedProjectDialog = () =>
	useContext(MoveToUnpinnedProjectContext);

/**
 * Holds the dialog above every menu that moves a chat, since a menu unmounts
 * before its move finishes.
 */
export const MoveToUnpinnedProjectDialogProvider: React.FC<
	React.PropsWithChildren
> = ({ children }) => {
	// The project stays after closing so the exit animation keeps its text.
	const [dialog, setDialog] = useState<{
		project?: ChatProject;
		open: boolean;
	}>({ open: false });
	return (
		<MoveToUnpinnedProjectContext
			value={(project) => setDialog({ project, open: true })}
		>
			{children}
			{dialog.project && (
				<MoveToUnpinnedProjectDialog
					project={dialog.project}
					open={dialog.open}
					onClose={() => setDialog((current) => ({ ...current, open: false }))}
				/>
			)}
		</MoveToUnpinnedProjectContext>
	);
};

type MoveToUnpinnedProjectDialogProps = {
	readonly project: ChatProject;
	readonly open: boolean;
	readonly onClose: () => void;
};

/** Explains that a moved chat left the sidebar, and offers to pin its project. */
export const MoveToUnpinnedProjectDialog: React.FC<
	MoveToUnpinnedProjectDialogProps
> = ({ project, open, onClose }) => {
	const queryClient = useQueryClient();
	const navigate = useNavigate();
	const location = useLocation();
	const pinMutation = useMutation({
		...pinChatProject(queryClient),
		onSuccess: onClose,
		onError: (error) => {
			toast.error(getErrorMessage(error, "Failed to pin project."));
		},
	});
	return (
		<Dialog open={open} onOpenChange={(next) => !next && onClose()}>
			<DialogContent>
				<DialogHeader>
					<DialogTitle>Chat moved to {project.name}</DialogTitle>
					<DialogDescription>
						Chats in {project.name} appear in the sidebar only while you have
						the project pinned. Pin it to keep this chat in the sidebar, or find
						the chat on the project page.
					</DialogDescription>
				</DialogHeader>
				<DialogFooter>
					<Button
						variant="outline"
						onClick={() => {
							onClose();
							navigate({
								pathname: buildAgentProjectPath({ projectId: project.id }),
								search: location.search,
							});
						}}
					>
						Go to project
					</Button>
					<Button
						disabled={pinMutation.isPending}
						onClick={() => pinMutation.mutate({ project })}
					>
						<Spinner loading={pinMutation.isPending} />
						Pin project
					</Button>
				</DialogFooter>
			</DialogContent>
		</Dialog>
	);
};
