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
import { type FC, useId, useState } from "react";

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
					<DialogTitle>Archive Chat</DialogTitle>
					<DialogDescription>
						Are you sure you want to archive{" "}
						<span className="font-semibold text-content-primary">
							{chatTitle}
						</span>
						? This chat has an associated workspace.
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
						onClick={onArchiveOnly}
					>
						<Spinner loading={isLoading} size="sm" />
						<ArchiveIcon />
						Archive only
					</Button>
					<Button
						variant="destructive"
						disabled={!deleteWorkspace || isLoading}
						onClick={onArchiveAndDeleteWorkspace}
					>
						<Spinner loading={isLoading} size="sm" />
						<Trash2Icon />
						Archive &amp; Delete Workspace
					</Button>
				</DialogFooter>
			</DialogContent>
		</Dialog>
	);
};
