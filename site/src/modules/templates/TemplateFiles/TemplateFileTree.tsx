import { ChevronDownIcon, ChevronRightIcon } from "lucide-react";
import { type CSSProperties, type FC, type JSX, useState } from "react";
import {
	Collapsible,
	CollapsibleContent,
	CollapsibleTrigger,
} from "#/components/Collapsible/Collapsible";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuTrigger,
} from "#/components/DropdownMenu/DropdownMenu";
import { cn } from "#/utils/cn";
import type { FileTree } from "#/utils/filetree";
import { getTemplateFileIcon } from "./TemplateFileIcon";

const isFolder = (content?: FileTree | string): content is FileTree =>
	typeof content === "object";

type FileTreeEntry = [key: string, content: FileTree | string];
function compareFileTreeEntries(
	[keyA, contentA]: FileTreeEntry,
	[keyB, contentB]: FileTreeEntry,
) {
	// A and B are either both files or both folders, so they should be sorted
	// lexically.
	if (isFolder(contentA) === isFolder(contentB)) {
		return keyA.localeCompare(keyB);
	}
	// Either A or B is a folder, and the other is a file. Put whichever one
	// is a folder first.
	return isFolder(contentA) ? -1 : 1;
}

type ContextMenu = {
	path: string;
	clientX: number;
	clientY: number;
};

interface TemplateFilesTreeProps {
	onSelect: (path: string) => void;
	onDelete?: (path: string) => void;
	onRename?: (path: string) => void;
	fileTree: FileTree;
	activePath?: string;
	Label?: FC<{
		path: string;
		filename: string;
		label: string;
		isFolder: boolean;
	}>;
}

export const TemplateFileTree: FC<TemplateFilesTreeProps> = ({
	fileTree,
	activePath,
	onDelete,
	onRename,
	onSelect,
	Label,
}) => {
	const [contextMenu, setContextMenu] = useState<ContextMenu | undefined>();

	const defaultExpanded = activePath ? expandablePaths(activePath) : [];

	const buildTreeItems = (
		label: string,
		filename: string,
		content?: FileTree | string,
		parentPath?: string,
	): JSX.Element => {
		const currentPath = parentPath ? `${parentPath}/${filename}` : filename;
		// Used to group empty folders in one single label like VSCode does.
		const shouldGroupFolder =
			isFolder(content) &&
			Object.keys(content).length === 1 &&
			isFolder(Object.values(content)[0]);
		const isHiddenFile = currentPath.startsWith(".");

		if (shouldGroupFolder) {
			const firstChildFileName = Object.keys(content)[0];
			const child = content[firstChildFileName];
			return buildTreeItems(
				`${label} / ${firstChildFileName}`,
				firstChildFileName,
				child,
				currentPath,
			);
		}

		const Icon = getTemplateFileIcon(filename, isFolder(content));
		const isActive = currentPath === activePath;

		const labelContent = Label ? (
			<Label
				path={currentPath}
				label={label}
				filename={filename}
				isFolder={isFolder(content)}
			/>
		) : (
			label
		);

		const handleContextMenu = (event: React.MouseEvent) => {
			const hasContextActions = onRename || onDelete;
			if (!hasContextActions) {
				return;
			}
			event.preventDefault();
			event.stopPropagation();
			setContextMenu(
				contextMenu
					? undefined
					: {
							path: currentPath,
							clientY: event.clientY,
							clientX: event.clientX,
						},
			);
		};

		if (isFolder(content)) {
			return (
				<FolderNode
					key={currentPath}
					defaultOpen={defaultExpanded.includes(currentPath)}
					label={labelContent}
					icon={<Icon className="size-3 shrink-0 text-current" />}
					isHidden={isHiddenFile}
					isActive={isActive}
					depth={parentPath ? parentPath.split("/").length : 0}
					onClick={() => onSelect(currentPath)}
					onContextMenu={handleContextMenu}
				>
					{Object.entries(content)
						.sort(compareFileTreeEntries)
						.map(([childName, child]) =>
							buildTreeItems(childName, childName, child, currentPath),
						)}
				</FolderNode>
			);
		}

		return (
			<FileNode
				key={currentPath}
				label={labelContent}
				icon={<Icon className="size-3 shrink-0 text-current" />}
				isHidden={isHiddenFile}
				isActive={isActive}
				depth={parentPath ? parentPath.split("/").length : 0}
				onClick={() => onSelect(currentPath)}
				onContextMenu={handleContextMenu}
			/>
		);
	};

	return (
		<div aria-label="Files" role="tree">
			{Object.entries(fileTree)
				.sort(compareFileTreeEntries)
				.map(([filename, child]) => buildTreeItems(filename, filename, child))}

			<DropdownMenu
				open={Boolean(contextMenu)}
				onOpenChange={(open) => {
					if (!open) {
						setContextMenu(undefined);
					}
				}}
			>
				{/* Hidden trigger positioned at the context menu coordinates. */}
				<DropdownMenuTrigger asChild>
					<span
						className="pointer-events-none fixed"
						style={
							contextMenu
								? {
										top: contextMenu.clientY,
										left: contextMenu.clientX,
									}
								: { top: -9999, left: -9999 }
						}
					/>
				</DropdownMenuTrigger>
				<DropdownMenuContent align="start">
					{onRename && (
						<DropdownMenuItem
							onClick={() => {
								if (!contextMenu) return;
								onRename(contextMenu.path);
								setContextMenu(undefined);
							}}
						>
							Rename
						</DropdownMenuItem>
					)}
					{onDelete && (
						<DropdownMenuItem
							onClick={() => {
								if (!contextMenu) return;
								onDelete(contextMenu.path);
								setContextMenu(undefined);
							}}
						>
							Delete
						</DropdownMenuItem>
					)}
				</DropdownMenuContent>
			</DropdownMenu>
		</div>
	);
};

interface TreeNodeProps {
	label: React.ReactNode;
	icon: React.ReactNode;
	isHidden: boolean;
	isActive: boolean;
	depth: number;
	onClick: () => void;
	onContextMenu: (event: React.MouseEvent) => void;
}

const nodeClasses =
	"flex h-8 cursor-pointer select-none items-center gap-1 border-none bg-transparent px-4 text-[13px] w-full text-left";

const FileNode: FC<TreeNodeProps> = ({
	label,
	icon,
	isHidden,
	isActive,
	depth,
	onClick,
	onContextMenu,
}) => {
	return (
		<button
			type="button"
			role="treeitem"
			className={cn(
				nodeClasses,
				isHidden ? "text-content-disabled" : "text-content-secondary",
				isActive && "bg-surface-sky text-content-link",
			)}
			style={{ paddingLeft: `${(depth + 1) * 8 + 8}px` } as CSSProperties}
			onClick={onClick}
			onContextMenu={onContextMenu}
		>
			{icon}
			<span className="truncate">{label}</span>
		</button>
	);
};

interface FolderNodeProps extends TreeNodeProps {
	defaultOpen: boolean;
	children: React.ReactNode;
}

const FolderNode: FC<FolderNodeProps> = ({
	defaultOpen,
	label,
	icon,
	isHidden,
	isActive,
	depth,
	onClick,
	onContextMenu,
	children,
}) => {
	const [open, setOpen] = useState(defaultOpen);

	return (
		<Collapsible open={open} onOpenChange={setOpen} role="treeitem">
			<CollapsibleTrigger asChild>
				<button
					type="button"
					className={cn(
						nodeClasses,
						isHidden ? "text-content-disabled" : "text-content-secondary",
						isActive && "bg-surface-sky text-content-link",
					)}
					style={
						{
							paddingLeft: `${(depth + 1) * 8 + 8}px`,
						} as CSSProperties
					}
					onClick={() => {
						// CollapsibleTrigger handles open/close toggling.
						// Fire onSelect so the parent knows the folder
						// was clicked.
						onClick();
					}}
					onContextMenu={onContextMenu}
				>
					{open ? (
						<ChevronDownIcon className="size-3 shrink-0" />
					) : (
						<ChevronRightIcon className="size-3 shrink-0" />
					)}
					{icon}
					<span className="truncate">{label}</span>
				</button>
			</CollapsibleTrigger>
			<CollapsibleContent role="group">{children}</CollapsibleContent>
		</Collapsible>
	);
};

const expandablePaths = (path: string) => {
	const paths = path.split("/");
	const result = [];
	for (let i = 1; i < paths.length; i++) {
		result.push(paths.slice(0, i).join("/"));
	}
	return result;
};
