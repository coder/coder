import {
	ChevronDownIcon,
	ChevronRightIcon,
	EllipsisIcon,
	PencilIcon,
	Trash2Icon,
} from "lucide-react";
import { type CSSProperties, type FC, type JSX, useState } from "react";
import { Button } from "#/components/Button/Button";
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
					onDelete={onDelete && (() => onDelete(currentPath))}
					onRename={onRename && (() => onRename(currentPath))}
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
				onDelete={onDelete && (() => onDelete(currentPath))}
				onRename={onRename && (() => onRename(currentPath))}
			/>
		);
	};

	return (
		<div>
			{Object.entries(fileTree)
				.sort(compareFileTreeEntries)
				.map(([filename, child]) => buildTreeItems(filename, filename, child))}
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
	onDelete?: () => void;
	onRename?: () => void;
}

const nodeClasses =
	"flex h-8 cursor-pointer select-none items-center gap-1 border-none bg-transparent px-4 text-[13px] w-full text-left hover:bg-surface-secondary focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-content-link focus-visible:ring-inset";

const FileNode: FC<TreeNodeProps> = ({
	label,
	icon,
	isHidden,
	isActive,
	depth,
	onClick,
	onDelete,
	onRename,
}) => {
	return (
		<div className="group/row relative flex items-center">
			<button
				type="button"
				className={cn(
					nodeClasses,
					isHidden ? "text-content-disabled" : "text-content-secondary",
					isActive && "bg-surface-sky text-content-link",
				)}
				style={{ paddingLeft: `${(depth + 1) * 8 + 8}px` } as CSSProperties}
				onClick={onClick}
			>
				{icon}
				<span className="truncate">{label}</span>
			</button>
			<MoreMenu onRename={onRename} onDelete={onDelete} />
		</div>
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
	onDelete,
	onRename,
	children,
}) => {
	const [open, setOpen] = useState(defaultOpen);

	return (
		<Collapsible open={open} onOpenChange={setOpen}>
			<div className="group/row relative flex items-center">
				<CollapsibleTrigger asChild>
					<button
						type="button"
						className={cn(
							nodeClasses,
							isHidden ? "text-content-disabled" : "text-content-secondary",
							isActive && "bg-surface-sky text-content-link",
						)}
						aria-expanded={open}
						style={
							{
								paddingLeft: `${(depth + 1) * 8 + 8}px`,
							} as CSSProperties
						}
						onClick={() => {
							// CollapsibleTrigger handles open/close toggling.
							// Also fire onClick so the caller is notified.
							onClick();
						}}
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
				<MoreMenu onRename={onRename} onDelete={onDelete} />
			</div>
			<CollapsibleContent>{children}</CollapsibleContent>
		</Collapsible>
	);
};

interface MoreMenuProps {
	onRename?: () => void;
	onDelete?: () => void;
}

const MoreMenu: FC<MoreMenuProps> = ({ onRename, onDelete }) => {
	if (!onRename && !onDelete) {
		return null;
	}

	return (
		<DropdownMenu>
			<DropdownMenuTrigger asChild>
				<Button
					size="icon"
					variant="subtle"
					className={cn(
						"absolute right-1 z-10 size-6 shrink-0",
						"opacity-0 transition-opacity",
						"group-hover/row:opacity-100",
						"focus-visible:opacity-100",
						"data-[state=open]:opacity-100",
					)}
					onClick={(e) => e.stopPropagation()}
				>
					<EllipsisIcon className="size-4" />
					<span className="sr-only">File actions</span>
				</Button>
			</DropdownMenuTrigger>
			<DropdownMenuContent align="end">
				{onRename && (
					<DropdownMenuItem onClick={onRename}>
						<PencilIcon />
						Rename
					</DropdownMenuItem>
				)}
				{onDelete && (
					<DropdownMenuItem
						className="text-content-destructive focus:text-content-destructive"
						onClick={onDelete}
					>
						<Trash2Icon />
						Delete&hellip;
					</DropdownMenuItem>
				)}
			</DropdownMenuContent>
		</DropdownMenu>
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
