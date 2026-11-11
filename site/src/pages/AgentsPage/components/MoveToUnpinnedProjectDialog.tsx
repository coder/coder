import { createContext, useContext, useRef, useState } from "react";
import { useMutation, useQueryClient } from "react-query";
import { useLocation, useNavigate } from "react-router";
import { getErrorMessage } from "#/api/errors";
import { pinChatProject } from "#/api/queries/chatProjects";
import type { ChatProject } from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import {
	Dialog,
	DialogClose,
	DialogContent,
	DialogDescription,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from "#/components/Dialog/Dialog";
import { Spinner } from "#/components/Spinner/Spinner";
import { toast } from "#/components/Toaster/toast";
import { focusFirstVisible } from "../utils/focus";
import { buildAgentProjectPath } from "../utils/navigation";

type MoveToUnpinnedProjectDialogControls = {
	readonly show: (project: ChatProject, chatId: string) => void;
	/**
	 * Registers what moves focus when the dialog closes and the moved chat has
	 * no actions button on screen. It returns whether an element took focus.
	 */
	readonly setFocusFallback: (focus: (() => boolean) | undefined) => void;
};

const MoveToUnpinnedProjectContext = createContext<
	MoveToUnpinnedProjectDialogControls | undefined
>(undefined);

/**
 * Controls the dialog that explains a chat moved into an unpinned project left
 * the sidebar. Undefined outside the provider, where no dialog opens.
 */
export const useMoveToUnpinnedProjectDialog = () =>
	useContext(MoveToUnpinnedProjectContext);

/**
 * Holds the dialog above every menu that moves a chat, since a menu unmounts
 * before its move finishes.
 */
export const MoveToUnpinnedProjectDialogProvider: React.FC<
	React.PropsWithChildren
> = ({ children }) => {
	// The project stays after closing so the exit animation keeps its text.
	// Each show gets its own ID, so a pin started from an earlier dialog
	// neither closes nor disables a later one.
	const [dialog, setDialog] = useState<{
		id: number;
		project?: ChatProject;
		chatId?: string;
		open: boolean;
	}>({ id: 0, open: false });
	const focusFallbackRef = useRef<(() => boolean) | undefined>(undefined);
	// The menu that moved the chat often unmounts with its row, or is still
	// closing when the move finishes, so Radix would return focus to the body.
	// A small screen keeps the sidebar's copy of the button mounted but hidden.
	const restoreFocus = (event: Event) => {
		const focused =
			focusFirstVisible([
				...document.querySelectorAll<HTMLElement>(
					`[data-chat-actions-for="${dialog.chatId}"]:not(:disabled)`,
				),
			]) ||
			(focusFallbackRef.current?.() ?? false);
		if (focused) {
			event.preventDefault();
		}
	};
	return (
		<MoveToUnpinnedProjectContext
			value={{
				show: (project, chatId) =>
					setDialog((current) => ({
						id: current.id + 1,
						project,
						chatId,
						open: true,
					})),
				setFocusFallback: (focus) => {
					focusFallbackRef.current = focus;
				},
			}}
		>
			{children}
			{dialog.project && (
				<MoveToUnpinnedProjectDialog
					key={dialog.id}
					project={dialog.project}
					open={dialog.open}
					onClose={() =>
						setDialog((current) =>
							current.id === dialog.id ? { ...current, open: false } : current,
						)
					}
					onCloseAutoFocus={restoreFocus}
				/>
			)}
		</MoveToUnpinnedProjectContext>
	);
};

type MoveToUnpinnedProjectDialogProps = {
	readonly project: ChatProject;
	readonly open: boolean;
	readonly onClose: () => void;
	readonly onCloseAutoFocus?: (event: Event) => void;
};

/** Explains that a moved chat left the sidebar, and offers to pin its project. */
export const MoveToUnpinnedProjectDialog: React.FC<
	MoveToUnpinnedProjectDialogProps
> = ({ project, open, onClose, onCloseAutoFocus }) => {
	const queryClient = useQueryClient();
	const navigate = useNavigate();
	const location = useLocation();
	const pinOptions = pinChatProject(queryClient);
	const pinMutation = useMutation({
		...pinOptions,
		onError: (error, variables, context) => {
			pinOptions.onError?.(error, variables, context);
			toast.error(getErrorMessage(error, "Failed to pin project."));
		},
	});
	return (
		<Dialog open={open} onOpenChange={(next) => !next && onClose()}>
			<DialogContent onCloseAutoFocus={onCloseAutoFocus}>
				<DialogHeader>
					<DialogTitle>Chat moved to {project.name}</DialogTitle>
					<DialogDescription>
						This chat appears in the sidebar only while {project.name} is
						pinned. Pin the project to keep it there, or find the chat on the
						project page.
					</DialogDescription>
				</DialogHeader>
				<DialogFooter>
					<DialogClose asChild>
						<Button variant="subtle">Close</Button>
					</DialogClose>
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
						// Closing once the sidebar has refetched lets focus return to the
						// chat in its project's folder.
						onClick={() =>
							pinMutation.mutate({ project }, { onSuccess: onClose })
						}
					>
						<Spinner loading={pinMutation.isPending} />
						Pin project
					</Button>
				</DialogFooter>
			</DialogContent>
		</Dialog>
	);
};
