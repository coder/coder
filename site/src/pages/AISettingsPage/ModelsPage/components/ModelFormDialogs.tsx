import { TriangleAlertIcon } from "lucide-react";
import type { FC } from "react";
import type * as TypesGen from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import { ConfirmDialog } from "#/components/Dialog/ConfirmDialog/ConfirmDialog";
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from "#/components/Dialog/Dialog";
import type { ModelFormValues } from "#/pages/AgentsPage/components/ChatModelAdminPanel/modelConfigFormLogic";

export const ModelFormDialogs: FC<{
	editingModel?: TypesGen.ChatModel;
	onDeleteModel?: (modelId: string) => Promise<void>;
	isDeleting: boolean;
	confirmingDelete: boolean;
	setConfirmingDelete: (open: boolean) => void;
	resetForm: (values: ModelFormValues) => void;
	formValues: ModelFormValues;
	unsavedChanges: {
		isOpen: boolean;
		onCancel: () => void;
		onConfirm: () => void;
	};
	confirmingReplaceDefault: boolean;
	setConfirmingReplaceDefault: (open: boolean) => void;
	currentDefaultModel?: TypesGen.ChatModel;
	onConfirmReplaceDefault: () => void;
}> = ({
	editingModel,
	onDeleteModel,
	isDeleting,
	confirmingDelete,
	setConfirmingDelete,
	resetForm,
	formValues,
	unsavedChanges,
	confirmingReplaceDefault,
	setConfirmingReplaceDefault,
	currentDefaultModel,
	onConfirmReplaceDefault,
}) => {
	return (
		<>
			{editingModel && onDeleteModel && (
				<ConfirmDialog
					type="delete"
					title="Delete model"
					confirmText="Delete model"
					description="Are you sure you want to delete this model? This action is irreversible."
					confirmLoading={isDeleting}
					open={confirmingDelete}
					onClose={() => setConfirmingDelete(false)}
					onConfirm={() => {
						resetForm(formValues);
						void onDeleteModel(editingModel.id);
					}}
				/>
			)}
			<Dialog
				open={unsavedChanges.isOpen}
				onOpenChange={(open) => !open && unsavedChanges.onCancel()}
			>
				<DialogContent className="border-border-warning">
					<DialogHeader>
						<DialogTitle>Unsaved changes</DialogTitle>
						<DialogDescription className="flex items-start gap-3">
							<TriangleAlertIcon className="size-icon-sm mt-1 shrink-0 text-content-primary" />
							<span>Your updates haven't been saved. Leave anyway?</span>
						</DialogDescription>
					</DialogHeader>
					<DialogFooter>
						<Button
							variant="outline"
							type="button"
							onClick={unsavedChanges.onCancel}
						>
							Cancel
						</Button>
						<Button type="button" onClick={unsavedChanges.onConfirm}>
							Confirm
						</Button>
					</DialogFooter>
				</DialogContent>
			</Dialog>
			<Dialog
				open={confirmingReplaceDefault}
				onOpenChange={(open) => !open && setConfirmingReplaceDefault(false)}
			>
				<DialogContent className="border-border-warning">
					<DialogHeader>
						<DialogTitle>Replace default model</DialogTitle>
						<DialogDescription className="flex items-center gap-2">
							<TriangleAlertIcon className="size-icon-sm shrink-0 text-content-primary" />
							<span>
								<strong className="text-content-primary">
									{currentDefaultModel?.display_name ||
										currentDefaultModel?.model}
								</strong>{" "}
								is currently the default. Replace it?
							</span>
						</DialogDescription>
					</DialogHeader>
					<DialogFooter>
						<Button
							variant="outline"
							type="button"
							onClick={() => setConfirmingReplaceDefault(false)}
						>
							Cancel
						</Button>
						<Button type="button" onClick={onConfirmReplaceDefault}>
							Confirm
						</Button>
					</DialogFooter>
				</DialogContent>
			</Dialog>
		</>
	);
};
