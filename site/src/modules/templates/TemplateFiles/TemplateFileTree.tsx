import { cn } from "cn";
import {
	EllipsisIcon,
	FolderIcon,
	FolderOpenIcon,
	PencilIcon,
	Trash2Icon,
} from "lucide-react";
import { useId, useImperativeHandle, useRef, useState } from "react";
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
			buildNode({
				label: filename,
				filename,
				content,
				pathPrefix: parentPath,
				depth,
				parentPath,
			}),
		);

type BuildNodeOptions = {
	label: string;
	filename: string;
	content: FileTree | string;
	/** Directory containing `filename`. Inside grouped folders it differs from `parentPath`. */
	pathPrefix?: string;
	depth: number;
	parentPath?: string;
};

const buildNode = ({
	label,
	filename,
	content,
	pathPrefix,
	depth,
	parentPath,
}: BuildNodeOptions): TreeNode => {
	const path = pathPrefix ? `${pathPrefix}/${filename}` : filename;

	if (!isFolder(content)) {
		return { path, label, filename, depth, parentPath };
	}

	// Folders whose only child is a folder are shown as one item, like VS Code.
	const childNames = Object.keys(content);
	if (childNames.length === 1 && isFolder(content[childNames[0]])) {
		const childName = childNames[0];
		return buildNode({
			label: `${label} / ${childName}`,
			filename: childName,
			content: content[childName],
			pathPrefix: path,
			depth,
			parentPath,
		});
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

const getTreeItemPath = (element: EventTarget | null) => {
	if (
		!(element instanceof Element) ||
		element.getAttribute("role") !== "treeitem"
	) {
		return undefined;
	}
	return element.getAttribute("data-path") ?? undefined;
};

const getNodeIcon = (node: TreeNode, isExpanded: boolean) => {
	if (!node.children) {
		return getTemplateFileIcon(node.filename);
	}
	return isExpanded ? FolderOpenIcon : FolderIcon;
};

/** Imperative handle for moving focus back into the tree, such as after a dialog closes. */
export type TemplateFileTreeHandle = {
	/** Focuses the tree's tab stop: the last focused, selected, or first item. */
	focus: () => void;
};

type TemplateFilesTreeProps = {
	ref?: React.Ref<TemplateFileTreeHandle>;
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

/** WAI-ARIA treeview. F2, Delete, and Shift+F10 run the row actions on the focused item. */
export const TemplateFileTree: React.FC<TemplateFilesTreeProps> = ({
	ref,
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

	useImperativeHandle(ref, () => ({
		focus: () => {
			if (tabStopPath !== undefined) {
				itemRefs.current.get(tabStopPath)?.focus();
			}
		},
	}));

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
		const path = getTreeItemPath(event.target);
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
		const item =
			event.target instanceof Element
				? event.target.closest("[role='treeitem']")
				: null;
		const path = getTreeItemPath(item);
		const node = path === undefined ? undefined : nodesByPath.get(path);
		if (node) {
			activate(node);
		}
	};

	const handleFocus = (event: React.FocusEvent<HTMLDivElement>) => {
		const path = getTreeItemPath(event.target);
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

	const tree: TreeState = {
		collapsed,
		activePath,
		tabStopPath,
		keyShortcuts: keyShortcuts || undefined,
		menuPath,
		onMenuPathChange: setMenuPath,
		onRename,
		onDelete,
		Label,
		registerItem: (path, element) => {
			if (element) {
				itemRefs.current.set(path, element);
			} else {
				itemRefs.current.delete(path);
			}
		},
	};

	return (
		<div
			role="tree"
			aria-label={ariaLabel}
			onKeyDown={handleKeyDown}
			onClick={handleClick}
			onFocus={handleFocus}
		>
			<TreeItems nodes={nodes} tree={tree} />
		</div>
	);
};

/** State and callbacks that every item in one tree shares. */
type TreeState = Pick<
	TemplateFilesTreeProps,
	"activePath" | "onRename" | "onDelete" | "Label"
> & {
	collapsed: ReadonlySet<string>;
	tabStopPath?: string;
	keyShortcuts?: string;
	menuPath?: string;
	onMenuPathChange: (path: string | undefined) => void;
	registerItem: (path: string, element: HTMLDivElement | null) => void;
};

type TreeItemsProps = {
	nodes: readonly TreeNode[];
	tree: TreeState;
};

const TreeItems: React.FC<TreeItemsProps> = ({ nodes, tree }) => {
	return nodes.map((node) => {
		const isExpanded =
			node.children !== undefined && !tree.collapsed.has(node.path);
		const Icon = getNodeIcon(node, isExpanded);
		return (
			<TreeItem
				key={node.path}
				node={node}
				tree={tree}
				isExpanded={isExpanded}
				icon={<Icon aria-hidden className="size-3 shrink-0 text-current" />}
			/>
		);
	});
};

type TreeItemProps = {
	node: TreeNode;
	tree: TreeState;
	isExpanded: boolean;
	icon: React.ReactNode;
};

const TreeItem: React.FC<TreeItemProps> = ({
	node,
	tree,
	isExpanded,
	icon,
}) => {
	const labelId = useId();
	const itemElement = useRef<HTMLDivElement | null>(null);
	const { Label, activePath, onRename, onDelete } = tree;
	const isFolderNode = node.children !== undefined;
	const isHidden = node.path.startsWith(".");
	const isSelected =
		activePath === undefined ? undefined : node.path === activePath;

	return (
		<div
			ref={(element) => {
				itemElement.current = element;
				tree.registerItem(node.path, element);
			}}
			role="treeitem"
			data-path={node.path}
			aria-labelledby={labelId}
			aria-expanded={isFolderNode ? isExpanded : undefined}
			aria-selected={isSelected}
			aria-keyshortcuts={tree.keyShortcuts}
			tabIndex={node.path === tree.tabStopPath ? 0 : -1}
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
						{Label ? (
							<Label
								path={node.path}
								label={node.label}
								filename={node.filename}
								isFolder={isFolderNode}
							/>
						) : (
							node.label
						)}
					</span>
				</div>
				<MoreMenu
					open={tree.menuPath === node.path}
					onOpenChange={(open) =>
						tree.onMenuPathChange(open ? node.path : undefined)
					}
					onRename={onRename && (() => onRename(node.path))}
					onDelete={onDelete && (() => onDelete(node.path))}
					onCloseAutoFocus={(event) => {
						event.preventDefault();
						itemElement.current?.focus();
					}}
				/>
			</div>
			{node.children && isExpanded && (
				<div role="group">
					<TreeItems nodes={node.children} tree={tree} />
				</div>
			)}
		</div>
	);
};

type MoreMenuProps = Required<
	Pick<React.ComponentProps<typeof DropdownMenu>, "open" | "onOpenChange">
> &
	Required<
		Pick<React.ComponentProps<typeof DropdownMenuContent>, "onCloseAutoFocus">
	> & {
		onRename?: () => void;
		onDelete?: () => void;
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
