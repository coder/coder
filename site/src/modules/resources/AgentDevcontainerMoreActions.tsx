import { Button } from "components/Button/Button";
import { ConfirmDialog } from "components/Dialogs/ConfirmDialog/ConfirmDialog";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuTrigger,
} from "components/DropdownMenu/DropdownMenu";
import { EllipsisVertical } from "lucide-react";
import { type FC, useId, useState } from "react";

type AgentDevcontainerMoreActionsProps = {
	deleteDevContainer: () => void;
};

export const AgentDevcontainerMoreActions: FC<
	AgentDevcontainerMoreActionsProps
> = ({ deleteDevContainer }) => {
	const [isConfirmingDelete, setIsConfirmingDelete] = useState(false);
	const [open, setOpen] = useState(false);
	const menuContentId = useId();

	return (
		<DropdownMenu open={open} onOpenChange={setOpen}>
			<DropdownMenuTrigger asChild>
				<Button size="icon-lg" variant="subtle" aria-controls={menuContentId}>
					<EllipsisVertical aria-hidden="true" />
					<span className="sr-only">Dev Container actions</span>
				</Button>
			</DropdownMenuTrigger>

			<DropdownMenuContent id={menuContentId} align="end">
				<DropdownMenuItem
					className="text-content-destructive focus:text-content-destructive"
					onClick={() => {
						setIsConfirmingDelete(true);
					}}
				>
					Delete&hellip;
				</DropdownMenuItem>
			</DropdownMenuContent>

			<DevcontainerDeleteDialog
				isOpen={isConfirmingDelete}
				onCancel={() => setIsConfirmingDelete(false)}
				onConfirm={() => {
					deleteDevContainer();
					setIsConfirmingDelete(false);
				}}
			/>
		</DropdownMenu>
	);
};

type DevcontainerDeleteDialogProps = {
	isOpen: boolean;
	onCancel: () => void;
	onConfirm: () => void;
};

const DevcontainerDeleteDialog: FC<DevcontainerDeleteDialogProps> = ({
	isOpen,
	onCancel,
	onConfirm,
}) => {
	return (
		<ConfirmDialog
			type="delete"
			open={isOpen}
			title="Delete dev container"
			onConfirm={onConfirm}
			onClose={onCancel}
			description={
				<p>
					Are you sure you want to delete this Dev Container? Any unsaved work
					will be lost.
				</p>
			}
		/>
	);
};
