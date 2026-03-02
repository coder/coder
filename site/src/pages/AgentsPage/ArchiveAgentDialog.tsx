import { Button } from "components/Button/Button";
import { Checkbox } from "components/Checkbox/Checkbox";
import {
	Dialog,
	DialogClose,
	DialogContent,
	DialogDescription,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from "components/Dialog/Dialog";
import { Spinner } from "components/Spinner/Spinner";
import { ArchiveIcon, Trash2Icon } from "lucide-react";
import { type FC, useEffect, useId, useState } from "react";

type LoadingAction = "archive-only" | "archive-and-delete" | null;

interface ArchiveAgentDialogProps {
	open: boolean;
	onClose: () => void;
	onArchiveOnly: () => void;
	onArchiveAndDeleteWorkspace: () => void;
	chatTitle: string;
	isLoading: boolean;
}

export const ArchiveAgentDialog: FC<ArchiveAgentDialogProps> = ({
	open,
	onClose,
	onArchiveOnly,
	onArchiveAndDeleteWorkspace,
	chatTitle,
	isLoading,
}) => {
	const checkboxId = useId();
	const [deleteWorkspace, setDeleteWorkspace] = useState(false);
	const [loadingAction, setLoadingAction] = useState<LoadingAction>(null);

	// Reset local state whenever the dialog opens so previous
	// selections don't carry over between different chats.
	useEffect(() => {
		if (open) {
			setDeleteWorkspace(false);
			setLoadingAction(null);
		}
	}, [open]);

	// Clear the loading action when the parent signals loading is done.
	useEffect(() => {
		if (!isLoading) {
			setLoadingAction(null);
		}
	}, [isLoading]);

	const handleArchiveOnly = () => {
		setLoadingAction("archive-only");
		onArchiveOnly();
	};

	const handleArchiveAndDelete = () => {
		setLoadingAction("archive-and-delete");
		onArchiveAndDeleteWorkspace();
	};

	const displayTitle = chatTitle || "this chat";

	return (
		<Dialog
			open={open}
			onOpenChange={(isOpen) => {
				if (!isOpen) {
					onClose();
				}
			}}
		>
			<DialogContent>
				<DialogHeader>
					<DialogTitle>Archive Agent</DialogTitle>
					<DialogDescription>
						Are you sure you want to archive{" "}
						<span className="font-semibold text-content-primary">
							{displayTitle}
						</span>
						? This agent has an associated workspace.
					</DialogDescription>
				</DialogHeader>

				<div className="flex items-center gap-2">
					<Checkbox
						id={checkboxId}
						checked={deleteWorkspace}
						onCheckedChange={(checked) => setDeleteWorkspace(checked === true)}
						disabled={isLoading}
					/>
					<label
						htmlFor={checkboxId}
						className="cursor-pointer select-none text-sm font-medium text-content-primary"
					>
						Also delete the associated workspace
					</label>
				</div>

				<DialogFooter>
					<DialogClose asChild>
						<Button variant="outline" disabled={isLoading}>
							Cancel
						</Button>
					</DialogClose>
					<Button
						variant="outline"
						disabled={isLoading}
						onClick={handleArchiveOnly}
					>
						{loadingAction === "archive-only" && <Spinner loading size="sm" />}
						<ArchiveIcon />
						Archive only
					</Button>
					<Button
						variant="destructive"
						disabled={!deleteWorkspace || isLoading}
						onClick={handleArchiveAndDelete}
					>
						{loadingAction === "archive-and-delete" && (
							<Spinner loading size="sm" />
						)}
						<Trash2Icon />
						Archive & Delete Workspace
					</Button>
				</DialogFooter>
			</DialogContent>
		</Dialog>
	);
};
