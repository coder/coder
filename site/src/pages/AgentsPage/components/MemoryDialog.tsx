import { type FC, useId, useState } from "react";
import { getErrorMessage } from "#/api/errors";
import { Button } from "#/components/Button/Button";
import {
	Dialog,
	DialogContent,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from "#/components/Dialog/Dialog";
import { Input } from "#/components/Input/Input";
import { Label } from "#/components/Label/Label";
import { Spinner } from "#/components/Spinner/Spinner";
import { Textarea } from "#/components/Textarea/Textarea";

const memoryNamePattern = /^[a-z0-9][a-z0-9_-]{0,63}$/;

export type MemoryRequest = {
	readonly name: string;
	readonly description: string;
	readonly body: string;
};

type MemoryDialogProps = {
	readonly memory?: (MemoryRequest & { readonly id: string }) | null;
	readonly open: boolean;
	readonly onOpenChange: (open: boolean) => void;
	readonly onSubmit: (request: MemoryRequest) => Promise<void>;
};

export const MemoryDialog: FC<MemoryDialogProps> = ({
	memory,
	open,
	onOpenChange,
	onSubmit,
}) => {
	const nameId = useId();
	const descriptionId = useId();
	const bodyId = useId();
	const [name, setName] = useState(memory?.name ?? "");
	const [description, setDescription] = useState(memory?.description ?? "");
	const [body, setBody] = useState(memory?.body ?? "");
	const [isSaving, setIsSaving] = useState(false);
	const [error, setError] = useState<string>();
	const isEditing = memory !== null && memory !== undefined;
	const isNameValid = memoryNamePattern.test(name);
	const isDescriptionValid =
		Boolean(description.trim()) && description.length <= 150;
	const isBodyValid = new TextEncoder().encode(body).length <= 8192;

	const handleOpenChange = (nextOpen: boolean) => {
		if (!nextOpen && !isSaving) {
			onOpenChange(false);
		}
	};

	const handleSubmit = async (event: React.FormEvent<HTMLFormElement>) => {
		event.preventDefault();
		if (!isNameValid || !isDescriptionValid || !isBodyValid || !body.trim()) {
			return;
		}
		setIsSaving(true);
		setError(undefined);
		await onSubmit({ name, description, body })
			.then(() => {
				onOpenChange(false);
			})
			.catch((submitError) => {
				setError(getErrorMessage(submitError, "Failed to save memory."));
			});
		setIsSaving(false);
	};

	return (
		<Dialog open={open} onOpenChange={handleOpenChange}>
			<DialogContent>
				<DialogHeader>
					<DialogTitle>{isEditing ? "Edit memory" : "Add memory"}</DialogTitle>
				</DialogHeader>
				<form className="flex flex-col gap-4" onSubmit={handleSubmit}>
					<div className="flex flex-col gap-2">
						<Label htmlFor={nameId}>Name</Label>
						<Input
							id={nameId}
							value={name}
							onChange={(event) => setName(event.target.value.toLowerCase())}
							disabled={isSaving}
							autoFocus
							aria-invalid={!isNameValid}
							aria-describedby={!isNameValid ? `${nameId}-error` : undefined}
						/>
						{!isNameValid && (
							<p
								id={`${nameId}-error`}
								className="m-0 text-xs text-content-destructive"
							>
								Use lowercase letters, numbers, underscores, or hyphens.
							</p>
						)}
					</div>
					<div className="flex flex-col gap-2">
						<div className="flex items-center justify-between gap-2">
							<Label htmlFor={descriptionId}>Description</Label>
							<span className="text-xs text-content-secondary">
								{description.length}/150
							</span>
						</div>
						<Input
							id={descriptionId}
							value={description}
							onChange={(event) => setDescription(event.target.value)}
							disabled={isSaving}
							maxLength={150}
							aria-invalid={!isDescriptionValid}
							aria-describedby={
								!isDescriptionValid ? `${descriptionId}-error` : undefined
							}
						/>
						{!isDescriptionValid && (
							<p
								id={`${descriptionId}-error`}
								className="m-0 text-xs text-content-destructive"
							>
								Enter a description.
							</p>
						)}
					</div>
					<div className="flex flex-col gap-2">
						<Label htmlFor={bodyId}>Body</Label>
						<Textarea
							id={bodyId}
							value={body}
							onChange={(event) => setBody(event.target.value)}
							disabled={isSaving}
							rows={8}
							aria-invalid={!isBodyValid}
						/>
					</div>
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
						<Button
							type="submit"
							disabled={
								!isNameValid ||
								!isDescriptionValid ||
								!isBodyValid ||
								!body.trim() ||
								isSaving
							}
						>
							<Spinner loading={isSaving} />
							Save
						</Button>
					</DialogFooter>
				</form>
			</DialogContent>
		</Dialog>
	);
};
