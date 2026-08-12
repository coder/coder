import { type ChangeEvent, type FC, useState } from "react";
import { ConfirmDialog } from "#/components/Dialog/ConfirmDialog/ConfirmDialog";
import { FormField } from "#/components/FormField/FormField";
import { type FileTree, isFolder, validatePath } from "#/utils/filetree";

interface CreateFileDialogProps {
	onClose: () => void;
	checkExists: (path: string) => boolean;
	onConfirm: (path: string) => void;
	open: boolean;
	fileTree: FileTree;
}

export const CreateFileDialog: FC<CreateFileDialogProps> = ({
	checkExists,
	onClose,
	onConfirm,
	open,
	fileTree,
}) => {
	const [pathValue, setPathValue] = useState("");
	const [error, setError] = useState<string>();
	const handleChange = (event: ChangeEvent<HTMLInputElement>) => {
		setPathValue(event.target.value);
	};
	const handleConfirm = () => {
		if (pathValue === "") {
			setError("You must enter a path!");
			return;
		}
		if (checkExists(pathValue)) {
			setError("File already exists");
			return;
		}

		const pathError = validatePath(pathValue, fileTree);
		if (pathError) {
			setError(pathError);
			return;
		}
		onConfirm(pathValue);
		setError(undefined);
		setPathValue("");
	};

	return (
		<ConfirmDialog
			open={open}
			onClose={() => {
				onClose();
				setError(undefined);
				setPathValue("");
			}}
			onConfirm={handleConfirm}
			hideCancel={false}
			type="success"
			cancelText="Cancel"
			confirmText="Create"
			title="Create File"
			description={
				<div className="flex flex-col gap-8">
					<p>
						Specify the path to a file to be created. This path can contain
						slashes too.
					</p>
					<FormField
						autoFocus
						onKeyDown={(event) => {
							if (event.key === "Enter") {
								handleConfirm();
							}
						}}
						field={{
							name: "file-path",
							id: "file-path",
							value: pathValue,
							onChange: handleChange,
							onBlur: () => {},
							error: Boolean(error),
							helperText: error,
						}}
						label="File Path"
						autoComplete="off"
						placeholder="example.tf"
					/>
				</div>
			}
		/>
	);
};

interface DeleteFileDialogProps {
	onClose: () => void;
	onConfirm: () => void;
	open: boolean;
	filename: string;
}

export const DeleteFileDialog: FC<DeleteFileDialogProps> = ({
	onClose,
	onConfirm,
	open,
	filename,
}) => {
	return (
		<ConfirmDialog
			type="delete"
			onClose={onClose}
			open={open}
			onConfirm={onConfirm}
			title="Delete File"
			description={
				<>
					Are you sure you want to delete <strong>{filename}</strong>? It will
					be deleted permanently.
				</>
			}
		/>
	);
};

interface RenameFileDialogProps {
	onClose: () => void;
	onConfirm: (filename: string) => void;
	checkExists: (path: string) => boolean;
	open: boolean;
	filename: string;
	fileTree: FileTree;
}

export const RenameFileDialog: FC<RenameFileDialogProps> = ({
	checkExists,
	onClose,
	onConfirm,
	open,
	filename,
	fileTree,
}) => {
	const [pathValue, setPathValue] = useState(filename);
	const [error, setError] = useState<string>();
	const handleChange = (event: ChangeEvent<HTMLInputElement>) => {
		setPathValue(event.target.value);
	};
	const handleConfirm = () => {
		if (pathValue === "") {
			setError("You must enter a path!");
			return;
		}
		if (checkExists(pathValue)) {
			setError("File already exists");
			return;
		}

		//Check if a folder is renamed to a file
		const [_, extension] = pathValue.split(".");
		if (isFolder(filename, fileTree) && extension) {
			setError(`A folder can't be renamed to a file.`);
			return;
		}
		const pathError = validatePath(pathValue, fileTree);
		if (pathError) {
			setError(pathError);
			return;
		}
		onConfirm(pathValue);
		setError(undefined);
		setPathValue("");
	};

	return (
		<ConfirmDialog
			open={open}
			onClose={() => {
				onClose();
				setError(undefined);
				setPathValue("");
			}}
			onConfirm={handleConfirm}
			hideCancel={false}
			type="success"
			cancelText="Cancel"
			confirmText="Rename"
			title="Rename File"
			description={
				<div className="flex flex-col gap-4">
					<p>
						Rename <strong>{filename}</strong> to something else. This path can
						contain slashes too!
					</p>
					<FormField
						autoFocus
						onKeyDown={(event) => {
							if (event.key === "Enter") {
								handleConfirm();
							}
						}}
						field={{
							name: "file-path",
							id: "file-path",
							value: pathValue,
							onChange: handleChange,
							onBlur: () => {},
							error: Boolean(error),
							helperText: error,
						}}
						label="File Path"
						autoComplete="off"
						placeholder={filename}
					/>
				</div>
			}
		/>
	);
};
