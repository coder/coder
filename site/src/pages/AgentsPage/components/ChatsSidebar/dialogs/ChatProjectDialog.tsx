import { type FC, useId, useState } from "react";
import { getErrorMessage } from "#/api/errors";
import type * as TypesGen from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import {
	Dialog,
	DialogContent,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from "#/components/Dialog/Dialog";
import { IconField } from "#/components/IconField/IconField";
import { Input } from "#/components/Input/Input";
import { Label } from "#/components/Label/Label";
import { Spinner } from "#/components/Spinner/Spinner";
import { Textarea } from "#/components/Textarea/Textarea";

type ChatProjectDialogProps = {
	readonly organizationId: string;
	readonly project?: TypesGen.ChatProject | null;
	readonly open: boolean;
	readonly onOpenChange: (open: boolean) => void;
	readonly onSubmit: (
		request:
			| TypesGen.CreateChatProjectRequest
			| TypesGen.UpdateChatProjectRequest,
	) => Promise<void>;
};

export const ChatProjectDialog: FC<ChatProjectDialogProps> = ({
	organizationId,
	project,
	open,
	onOpenChange,
	onSubmit,
}) => {
	const [isSaving, setIsSaving] = useState(false);
	const handleOpenChange = (nextOpen: boolean) => {
		if (!nextOpen && !isSaving) {
			onOpenChange(false);
		}
	};

	return (
		<Dialog open={open} onOpenChange={handleOpenChange}>
			{/* Radix unmounts the content when the dialog closes, so field
			    state kept inside it resets on every open without remounting
			    the dialog itself. */}
			<DialogContent>
				<ChatProjectForm
					organizationId={organizationId}
					project={project}
					isSaving={isSaving}
					onSavingChange={setIsSaving}
					onOpenChange={onOpenChange}
					onSubmit={onSubmit}
				/>
			</DialogContent>
		</Dialog>
	);
};

type ChatProjectFormProps = Omit<ChatProjectDialogProps, "open"> & {
	readonly isSaving: boolean;
	readonly onSavingChange: (saving: boolean) => void;
};

const ChatProjectForm: FC<ChatProjectFormProps> = ({
	organizationId,
	project,
	isSaving,
	onSavingChange,
	onOpenChange,
	onSubmit,
}) => {
	const nameId = useId();
	const descriptionId = useId();
	const [name, setName] = useState(project?.name ?? "");
	const [description, setDescription] = useState(project?.description ?? "");
	const [icon, setIcon] = useState(project?.icon ?? "");
	const [error, setError] = useState<string>();
	const isEditing = project !== null && project !== undefined;

	const handleSubmit = async (event: React.FormEvent<HTMLFormElement>) => {
		event.preventDefault();
		const trimmedName = name.trim();
		if (!trimmedName) {
			return;
		}
		onSavingChange(true);
		setError(undefined);
		await onSubmit(
			isEditing
				? {
						name: trimmedName,
						description: description.trim(),
						icon: icon.trim(),
					}
				: {
						organization_id: organizationId,
						name: trimmedName,
						description: description.trim(),
						icon: icon.trim(),
					},
		)
			.then(() => {
				onOpenChange(false);
			})
			.catch((submitError) => {
				setError(getErrorMessage(submitError, "Failed to save project."));
			});
		onSavingChange(false);
	};

	return (
		<>
			<DialogHeader>
				<DialogTitle>{isEditing ? "Edit project" : "New project"}</DialogTitle>
			</DialogHeader>
			<form className="flex flex-col gap-4" onSubmit={handleSubmit}>
				<div className="flex flex-col gap-2">
					<Label htmlFor={nameId}>Name</Label>
					<Input
						id={nameId}
						value={name}
						onChange={(event) => setName(event.target.value)}
						disabled={isSaving}
						maxLength={64}
						autoFocus
					/>
				</div>
				<div className="flex flex-col gap-2">
					<Label htmlFor={descriptionId}>Description</Label>
					<Textarea
						id={descriptionId}
						value={description}
						onChange={(event) => setDescription(event.target.value)}
						disabled={isSaving}
						maxLength={1024}
					/>
				</div>
				<IconField
					value={icon}
					onChange={(event) => setIcon(event.target.value)}
					onPickEmoji={setIcon}
					disabled={isSaving}
					maxLength={256}
				/>
				{error && (
					<p className="m-0 text-sm text-content-destructive">{error}</p>
				)}
				<DialogFooter>
					<Button
						type="button"
						variant="outline"
						disabled={isSaving}
						onClick={() => onOpenChange(false)}
					>
						Cancel
					</Button>
					<Button type="submit" disabled={!name.trim() || isSaving}>
						<Spinner loading={isSaving} />
						Save
					</Button>
				</DialogFooter>
			</form>
		</>
	);
};
