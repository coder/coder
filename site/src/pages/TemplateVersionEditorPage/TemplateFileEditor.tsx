import { MinusIcon, PlusIcon, TriangleAlertIcon } from "lucide-react";
import { useId, useRef, useState } from "react";
import { Button } from "#/components/Button/Button";
import { Sidebar } from "#/components/FullPageLayout/Sidebar";
import { isBinaryData } from "#/modules/templates/TemplateFiles/isBinaryData";
import { TemplateFileTree } from "#/modules/templates/TemplateFiles/TemplateFileTree";
import {
	createFile,
	existsFile,
	type FileTree,
	getFileText,
	isFolder,
	moveFile,
	removeFile,
	updateFile,
} from "#/utils/filetree";
import {
	CreateFileDialog,
	DeleteFileDialog,
	RenameFileDialog,
} from "./FileDialog";
import { MonacoEditor } from "./MonacoEditor";

type TemplateFileEditorProps = {
	fileTree: FileTree;
	onFileTreeChange: (updater: (files: FileTree) => FileTree) => void;
	activePath: string | undefined;
	onActivePathChange: (path: string | undefined) => void;
	children: React.ReactNode;
};

/** Shared archive editing surface for both Terraform roots. */
export const TemplateFileEditor: React.FC<TemplateFileEditorProps> = ({
	fileTree,
	onFileTreeChange,
	activePath,
	onActivePathChange,
	children,
}) => {
	const [creating, setCreating] = useState(false);
	const [deleting, setDeleting] = useState<string>();
	const [renaming, setRenaming] = useState<string>();
	const editorPanel = useRef<HTMLDivElement>(null);
	const editorId = useId();
	const value =
		activePath &&
		existsFile(activePath, fileTree) &&
		!isFolder(activePath, fileTree)
			? getFileText(activePath, fileTree)
			: undefined;
	const resizeEditor = (delta: number) => {
		const panel = editorPanel.current;
		if (panel)
			panel.style.height = `${panel.getBoundingClientRect().height + delta}px`;
	};

	return (
		<div className="flex flex-1 min-w-0 min-h-0 flex-col sm:flex-row">
			<Sidebar className="flex flex-col gap-px w-full sm:w-64 border-solid border-0 border-b sm:border-b-0 sm:border-r border-border py-2 shrink-0 max-h-40 sm:max-h-none sm:h-full overflow-y-auto">
				<div className="h-[42px] py-0 pr-2 pl-4 flex items-center justify-between">
					<span className="text-content-primary text-sm">Files</span>
					<Button
						size="icon"
						variant="subtle"
						aria-label="Create File"
						onClick={() => setCreating(true)}
					>
						<PlusIcon />
					</Button>
				</div>
				<TemplateFileTree
					fileTree={fileTree}
					activePath={activePath}
					onDelete={setDeleting}
					onRename={setRenaming}
					onSelect={(path) => {
						if (!isFolder(path, fileTree)) onActivePathChange(path);
					}}
				/>
			</Sidebar>
			<div className="flex flex-col flex-1 min-w-0 min-h-0 overflow-hidden">
				<div className="flex items-center justify-end gap-1 border-0 border-b border-solid border-border px-2 text-xs text-content-secondary shrink-0">
					<span className="mr-auto truncate">
						{activePath ?? "No file opened"}
					</span>
					<span>Editor height</span>
					<Button
						variant="subtle"
						size="icon"
						aria-label="Decrease editor height"
						aria-controls={editorId}
						onClick={() => resizeEditor(-80)}
					>
						<MinusIcon />
					</Button>
					<Button
						variant="subtle"
						size="icon"
						aria-label="Increase editor height"
						aria-controls={editorId}
						onClick={() => resizeEditor(80)}
					>
						<PlusIcon />
					</Button>
				</div>
				<div
					ref={editorPanel}
					id={editorId}
					className="relative h-[60%] min-h-40 max-h-[calc(100%-100px)] shrink-0 resize-y overflow-hidden pb-2"
					data-pixel="ignore"
				>
					<div className="absolute inset-x-0 top-0 bottom-2">
						{value === undefined ? (
							<div className="p-4">
								Select or create a file to start editing.
							</div>
						) : isBinaryData(value) ? (
							<div
								role="alert"
								className="h-full flex items-center justify-center gap-3 p-10"
							>
								<TriangleAlertIcon className="text-content-warning shrink-0" />
								The file is not displayed in the text editor because it is
								either binary or uses an unsupported text encoding.
							</div>
						) : (
							<MonacoEditor
								value={value}
								path={activePath}
								onChange={(content) => {
									if (activePath && content !== value)
										onFileTreeChange((files) =>
											updateFile(activePath, content, files),
										);
								}}
							/>
						)}
					</div>
				</div>
				<div className="flex-1 min-h-0 overflow-auto border-0 border-t border-solid border-border">
					{children}
				</div>
			</div>
			<CreateFileDialog
				fileTree={fileTree}
				open={creating}
				onClose={() => setCreating(false)}
				checkExists={(path) => existsFile(path, fileTree)}
				onConfirm={(path) => {
					onFileTreeChange((files) => createFile(path, files, ""));
					onActivePathChange(path);
					setCreating(false);
				}}
			/>
			<DeleteFileDialog
				open={Boolean(deleting)}
				filename={deleting ?? ""}
				onClose={() => setDeleting(undefined)}
				onConfirm={() => {
					if (!deleting) return;
					onFileTreeChange((files) => removeFile(deleting, files));
					if (activePath === deleting) onActivePathChange(undefined);
					setDeleting(undefined);
				}}
			/>
			<RenameFileDialog
				fileTree={fileTree}
				open={Boolean(renaming)}
				filename={renaming ?? ""}
				onClose={() => setRenaming(undefined)}
				checkExists={(path) => existsFile(path, fileTree)}
				onConfirm={(path) => {
					if (!renaming) return;
					onFileTreeChange((files) => moveFile(renaming, path, files));
					onActivePathChange(path);
					setRenaming(undefined);
				}}
			/>
		</div>
	);
};
