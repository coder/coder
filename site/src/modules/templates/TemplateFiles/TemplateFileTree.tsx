import { cn } from "cn";
import {
	EllipsisIcon,
	FolderIcon,
	FolderOpenIcon,
	PencilIcon,
	Trash2Icon,
} from "lucide-react";
import { useId, useRef, useState } from "react";
import { Button } from "#/components/Button/Button";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuTrigger,
} from "#/components/DropdownMenu/DropdownMenu";
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
	// Either A or B is a folder, and the other is a file. Put whichever one is a
	// folder first.
	return isFolder(contentA) ? -1 : 1;
}

type TreeNode = {
	path: string;
	label: string;
	filename: string;
	depth: number;
	/** Path of the visible parent item, used for ArrowLeft navigation. */
	parentPath?: string;
	children?: TreeNode[];
};

const buildNodes = (
	fileTree: FileTree,
	parentPath?: string,
	depth = 0,
): TreeNode[] =>
	Object.entries(fileTree)
		.sort(compareFileTreeEntries)
		.map(([filename, content]) =>
			buildNode(filename, filename, content, parentPath, depth, parentPath),
		);

const buildNode = (
	label: string,
	filename: string,
	content: FileTree | string,
	pathPrefix: string | undefined,
	depth: number,
	parentPath: string | undefined,
): TreeNode => {
	const path = pathPrefix ? `${pathPrefix}/${filename}` : filename;

	if (!isFolder(content)) {
		return { path, label, filename, depth, parentPath };
	}

	// Folders whose only child is a folder are shown as one item, like VS Code.
	const childNames = Object.keys(content);
	if (childNames.length === 1 && isFolder(content[childNames[0]])) {
		const childName = childNames[0];
		return buildNode(
			`${label} / ${childName}`,
			childName,
			content[childName],
			path,
			depth,
			parentPath,
		);
	}

	return {
		path,
		label,
		filename,
		depth,
		parentPath,
		children: buildNodes(content, path, depth + 1),
	};
};

const flattenVisible = (
	nodes: readonly TreeNode[],
	collapsed: ReadonlySet<string>,
): TreeNode[] =>
	nodes.flatMap((node) =>
		node.children && !collapsed.has(node.path)
			? [node, ...flattenVisible(node.children, collapsed)]
			: [node],
	);

const getTreeItemPath = (target: EventTarget, closest: boolean) => {
	if (!(target instanceof Element)) {
		return undefined;
	}
	const item = closest ? target.closest("[role='treeitem']") : target;
	if (item?.getAttribute("role") !== "treeitem") {
		return undefined;
	}
	return item.getAttribute("data-path") ?? undefined;
};

type TemplateFilesTreeProps = {
	onSelect: (path: string) => void;
	onDelete?: (path: string) => void;
	onRename?: (path: string) => void;
	fileTree: FileTree;
	activePath?: string;
	"aria-label"?: string;
	Label?: React.FC<{
		path: string;
		filename: string;
		label: string;
		isFolder: boolean;
	}>;
};

/**
 * File tree following the WAI-ARIA treeview pattern: one tab stop, arrow keys
 * to move and expand, Enter or Space to select. F2, Delete, and Shift+F10
 * trigger the row actions when they are available.
 */
export const TemplateFileTree: React.FC<TemplateFilesTreeProps> = ({
	fileTree,
	activePath,
	onDelete,
	onRename,
	onSelect,
	Label,
	"aria-label": ariaLabel = "Template files",
}) => {
	const [collapsed, setCollapsed] = useState<ReadonlySet<string>>(
		() => new Set(),
	);
	const [focusedPath, setFocusedPath] = useState<string>();
	const [menuPath, setMenuPath] = useState<string>();
	const itemRefs = useRef(new Map<string, HTMLDivElement>());

	const nodes = buildNodes(fileTree);
	const visibleNodes = flattenVisible(nodes, collapsed);
	const nodesByPath = new Map(visibleNodes.map((node) => [node.path, node]));
	const hasActions = Boolean(onRename || onDelete);

	const tabStopPath =
		[focusedPath, activePath].find(
			(path) => path !== undefined && nodesByPath.has(path),
		) ?? visibleNodes[0]?.path;

	const setExpanded = (path: string, expanded: boolean) => {
		setCollapsed((prev) => {
			const next = new Set(prev);
			if (expanded) {
				next.delete(path);
			} else {
				next.add(path);
			}
			return next;
		});
	};

	const activate = (node: TreeNode) => {
		if (node.children) {
			setExpanded(node.path, collapsed.has(node.path));
		}
		onSelect(node.path);
	};

	const focusItem = (node: TreeNode | undefined) => {
		if (node) {
			itemRefs.current.get(node.path)?.focus();
		}
	};

	const handleKeyDown = (event: React.KeyboardEvent<HTMLDivElement>) => {
		const path = getTreeItemPath(event.target, false);
		const node = path === undefined ? undefined : nodesByPath.get(path);
		if (!node) {
			return;
		}
		const index = visibleNodes.indexOf(node);
		const isExpanded = node.children !== undefined && !collapsed.has(node.path);

		switch (event.key) {
			case "ArrowDown":
				focusItem(visibleNodes[index + 1]);
				break;
			case "ArrowUp":
				focusItem(visibleNodes[index - 1]);
				break;
			case "Home":
				focusItem(visibleNodes[0]);
				break;
			case "End":
				focusItem(visibleNodes.at(-1));
				break;
			case "ArrowRight":
				if (!node.children) {
					break;
				}
				if (isExpanded) {
					focusItem(node.children[0]);
				} else {
					setExpanded(node.path, true);
				}
				break;
			case "ArrowLeft":
				if (isExpanded) {
					setExpanded(node.path, false);
				} else if (node.parentPath !== undefined) {
					focusItem(nodesByPath.get(node.parentPath));
				}
				break;
			case "Enter":
			case " ":
				activate(node);
				break;
			case "F2":
				if (!onRename) {
					return;
				}
				onRename(node.path);
				break;
			case "Delete":
				if (!onDelete) {
					return;
				}
				onDelete(node.path);
				break;
			case "F10":
			case "ContextMenu":
				if (!hasActions || (event.key === "F10" && !event.shiftKey)) {
					return;
				}
				setMenuPath(node.path);
				break;
			default:
				return;
		}
		event.preventDefault();
	};

	const handleClick = (event: React.MouseEvent<HTMLDivElement>) => {
		const path = getTreeItemPath(event.target, true);
		const node = path === undefined ? undefined : nodesByPath.get(path);
		if (node) {
			activate(node);
		}
	};

	const handleFocus = (event: React.FocusEvent<HTMLDivElement>) => {
		const path = getTreeItemPath(event.target, false);
		if (path !== undefined) {
			setFocusedPath(path);
		}
	};

	const keyShortcuts = [
		onRename && "F2",
		onDelete && "Delete",
		hasActions && "Shift+F10",
	]
		.filter(Boolean)
		.join(" ");

	const renderNodes = (nodes: readonly TreeNode[]): React.ReactNode =>
		nodes.map((node) => {
			const isExpanded =
				node.children !== undefined && !collapsed.has(node.path);
			const Icon = node.children
				? isExpanded
					? FolderOpenIcon
					: FolderIcon
				: getTemplateFileIcon(node.filename);
			return (
				<TreeItem
					key={node.path}
					node={node}
					icon={<Icon aria-hidden className="size-3 shrink-0 text-current" />}
					labelContent={
						Label ? (
							<Label
								path={node.path}
								label={node.label}
								filename={node.filename}
								isFolder={node.children !== undefined}
							/>
						) : (
							node.label
						)
					}
					isExpanded={isExpanded}
					isSelected={
						activePath === undefined ? undefined : node.path === activePath
					}
					isTabStop={node.path === tabStopPath}
					keyShortcuts={keyShortcuts || undefined}
					menuOpen={menuPath === node.path}
					onMenuOpenChange={(open) => setMenuPath(open ? node.path : undefined)}
					onRename={onRename && (() => onRename(node.path))}
					onDelete={onDelete && (() => onDelete(node.path))}
					itemRef={(element) => {
						if (element) {
							itemRefs.current.set(node.path, element);
						} else {
							itemRefs.current.delete(node.path);
						}
					}}
				>
					{node.children && renderNodes(node.children)}
				</TreeItem>
			);
		});

	return (
		<div
			role="tree"
			aria-label={ariaLabel}
			onKeyDown={handleKeyDown}
			onClick={handleClick}
			onFocus={handleFocus}
		>
			{renderNodes(nodes)}
		</div>
	);
};

type TreeItemProps = {
	node: TreeNode;
	icon: React.ReactNode;
	labelContent: React.ReactNode;
	isExpanded: boolean;
	isSelected?: boolean;
	isTabStop: boolean;
	keyShortcuts?: string;
	menuOpen: boolean;
	onMenuOpenChange: (open: boolean) => void;
	onRename?: () => void;
	onDelete?: () => void;
	itemRef: React.RefCallback<HTMLDivElement>;
	children?: React.ReactNode;
};

const TreeItem: React.FC<TreeItemProps> = ({
	node,
	icon,
	labelContent,
	isExpanded,
	isSelected,
	isTabStop,
	keyShortcuts,
	menuOpen,
	onMenuOpenChange,
	onRename,
	onDelete,
	itemRef,
	children,
}) => {
	const labelId = useId();
	const itemElement = useRef<HTMLDivElement | null>(null);
	const isFolderNode = node.children !== undefined;
	const isHidden = node.path.startsWith(".");

	return (
		<div
			ref={(element) => {
				itemElement.current = element;
				itemRef(element);
			}}
			role="treeitem"
			data-path={node.path}
			aria-labelledby={labelId}
			aria-expanded={isFolderNode ? isExpanded : undefined}
			aria-selected={isSelected}
			aria-keyshortcuts={keyShortcuts}
			tabIndex={isTabStop ? 0 : -1}
			className="outline-hidden"
		>
			<div
				className={cn(
					"group/tree-item flex flex-row items-center justify-between",
					"hover:bg-surface-secondary",
					"[[role=treeitem]:focus-visible>&]:ring-2 [[role=treeitem]:focus-visible>&]:ring-inset",
					"[[role=treeitem]:focus-visible>&]:ring-content-link",
					isSelected && "bg-surface-sky",
				)}
			>
				<div
					className={cn(
						"grow flex h-8 min-w-0 cursor-pointer select-none items-center gap-2 px-4 text-sm",
						isHidden ? "text-content-secondary" : "text-content-primary",
						isSelected && "text-content-link",
					)}
					style={{ paddingLeft: `${(node.depth + 1) * 8 + 8}px` }}
				>
					{icon}
					<span id={labelId} className="truncate">
						{labelContent}
					</span>
				</div>
				<MoreMenu
					open={menuOpen}
					onOpenChange={onMenuOpenChange}
					onRename={onRename}
					onDelete={onDelete}
					onCloseAutoFocus={(event) => {
						event.preventDefault();
						itemElement.current?.focus();
					}}
				/>
			</div>
			{isFolderNode && isExpanded && <div role="group">{children}</div>}
		</div>
	);
};

type MoreMenuProps = {
	open: boolean;
	onOpenChange: (open: boolean) => void;
	onRename?: () => void;
	onDelete?: () => void;
	onCloseAutoFocus: (event: Event) => void;
};

const MoreMenu: React.FC<MoreMenuProps> = ({
	open,
	onOpenChange,
	onRename,
	onDelete,
	onCloseAutoFocus,
}) => {
	if (!onRename && !onDelete) {
		return null;
	}

	return (
		<DropdownMenu open={open} onOpenChange={onOpenChange}>
			{/* Pointer-only trigger. Keyboard users open the menu from the focused
			    tree item with Shift+F10, so the button stays out of the tab order
			    and the accessibility tree. */}
			<DropdownMenuTrigger asChild>
				<Button
					size="icon"
					variant="subtle"
					tabIndex={-1}
					aria-hidden
					className={cn(
						"size-6 shrink-0",
						"opacity-0 transition-opacity",
						"group-hover/tree-item:opacity-100",
						"data-[state=open]:opacity-100",
					)}
					onClick={(e) => e.stopPropagation()}
				>
					<EllipsisIcon className="size-4" />
				</Button>
			</DropdownMenuTrigger>
			<DropdownMenuContent align="end" onCloseAutoFocus={onCloseAutoFocus}>
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
