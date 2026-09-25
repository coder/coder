import type { FC, FormEvent } from "react";
import { Alert } from "#/components/Alert/Alert";
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
import {
	DeleteConfirmationField,
	useDeleteConfirmation,
} from "./DeleteConfirmationField";

type DeleteDialogProps = {
	isOpen: boolean;
	onConfirm: () => void;
	onCancel: () => void;
	entity: string;
	name: string;
	info?: string;
	confirmLoading?: boolean;
	verb?: string;
	title?: string;
	label?: string;
	confirmText?: string;
};

export const DeleteDialog: FC<DeleteDialogProps> = ({
	isOpen,
	onCancel,
	onConfirm,
	entity,
	info,
	name,
	confirmLoading = false,
	// Optional overrides for verbiage, e.g. "unlinking" vs "deleting".
	verb,
	title,
	label,
	confirmText = "Delete",
}) => {
	const confirmation = useDeleteConfirmation(name);

	const handleOpenChange = (open: boolean) => {
		if (!open) {
			confirmation.reset();
			onCancel();
		}
	};

	const onSubmit = (event: FormEvent<HTMLFormElement>) => {
		event.preventDefault();
		if (confirmation.confirmed && !confirmLoading) {
			onConfirm();
		}
	};

	return (
		<Dialog open={isOpen} onOpenChange={handleOpenChange}>
			<DialogContent variant="destructive" data-testid="dialog">
				<DialogHeader>
					<DialogTitle>{title ?? `Delete ${entity}`}</DialogTitle>
					<DialogDescription>
						{verb ?? "Deleting"} this {entity} is irreversible!
					</DialogDescription>
				</DialogHeader>

				<div className="flex flex-col gap-3">
					{info && (
						<Alert severity="warning" prominent>
							{info}
						</Alert>
					)}
					<p className="m-0 text-sm text-content-secondary font-medium">
						Type <strong className="text-content-primary">{name}</strong> below
						to confirm.
					</p>
				</div>

				<form className="flex flex-col gap-6" onSubmit={onSubmit}>
					<DeleteConfirmationField
						confirmation={confirmation}
						label={label ?? `Name of the ${entity} to delete`}
						entity={entity}
					/>

					<DialogFooter>
						<Button
							type="button"
							variant="outline"
							disabled={confirmLoading}
							onClick={() => handleOpenChange(false)}
						>
							Cancel
						</Button>
						<Button
							type="submit"
							variant="destructive"
							disabled={!confirmation.confirmed || confirmLoading}
							data-testid="confirm-button"
						>
							<Spinner loading={confirmLoading} />
							{confirmText}
						</Button>
					</DialogFooter>
				</form>
			</DialogContent>
		</Dialog>
	);
};
