import type { FC, ReactNode } from "react";
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

interface ConfirmDeleteDialogProps {
	open: boolean;
	onOpenChange: (open: boolean) => void;
	/** The entity type being deleted, shown in the title and button. */
	entity: string;
	/**
	 * Optional description. Defaults to "Are you sure you want to
	 * delete this {entity}? This action is irreversible."
	 */
	description?: ReactNode;
	onConfirm: () => void;
	isPending?: boolean;
}

export const ConfirmDeleteDialog: FC<ConfirmDeleteDialogProps> = ({
	open,
	onOpenChange,
	entity,
	description,
	onConfirm,
	isPending = false,
}) => (
	<Dialog open={open} onOpenChange={onOpenChange}>
		<DialogContent variant="destructive">
			<DialogHeader>
				<DialogTitle>Delete {entity}</DialogTitle>
				<DialogDescription>
					{description ??
						`Are you sure you want to delete this ${entity}? This action is irreversible.`}
				</DialogDescription>
			</DialogHeader>
			<DialogFooter>
				<Button
					variant="outline"
					onClick={() => onOpenChange(false)}
					disabled={isPending}
				>
					Cancel
				</Button>
				<Button variant="destructive" onClick={onConfirm} disabled={isPending}>
					{isPending && <Spinner className="h-4 w-4" loading />}
					Delete {entity}
				</Button>
			</DialogFooter>
		</DialogContent>
	</Dialog>
);
