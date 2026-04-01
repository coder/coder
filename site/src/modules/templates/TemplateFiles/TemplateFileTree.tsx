import { css } from "@emotion/react";
import Menu from "@mui/material/Menu";
import MenuItem from "@mui/material/MenuItem";
import { SimpleTreeView, TreeItem } from "@mui/x-tree-view";
import { ChevronDownIcon, ChevronRightIcon } from "lucide-react";
import { type CSSProperties, type FC, type JSX, useState } from "react";
import type { FileTree } from "utils/filetree";
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

	const buildTreeItems = (
		label: string,
		filename: string,
		content?: FileTree | string,
		parentPath?: string,
	): JSX.Element => {
		const currentPath = parentPath ? `${parentPath}/${filename}` : filename;
		// Used to group empty folders in one single label like VSCode does
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

		const templateFileIcon = getTemplateFileIcon(filename, isFolder(content));

		return (
			<TreeItem
				slots={{ icon: templateFileIcon }}
				itemId={currentPath}
				key={currentPath}
				label={
					Label ? (
						<Label
							path={currentPath}
							label={label}
							filename={filename}
							isFolder={isFolder(content)}
						/>
					) : (
						label
					)
				}
				css={(theme) => css`
          overflow: hidden;
          user-select: none;

          & > .MuiTreeItem-content {
						border-radius: 0;
            padding: 2px 16px;
            color: ${
							isHiddenFile
								? theme.palette.text.disabled
								: theme.palette.text.secondary
						};
            height: 32px;

            & svg {
              width: 12px;
              height: 12px;
              color: currentColor;
            }

            & > .MuiTreeItem-label {
              margin-left: 4px;
              font-size: 13px;
              color: inherit;
              white-space: nowrap;
            }

            &.Mui-selected {
              color: ${theme.roles.active.text};
              background: ${theme.roles.active.background};
            }

            &.Mui-focused {
              box-shadow: inset 0 0 0 1px ${theme.palette.primary.main};
            }
          }

          & .MuiTreeItem-group {
            margin-left: 0;
            position: relative;

            // We need to find a better way to recursive padding here
            & .MuiTreeItem-content {
              padding-left: calc(8px + (var(--level) + 1) * 8px);
            }
          }
        `}
				onClick={() => {
					onSelect(currentPath);
				}}
				onContextMenu={(event) => {
					const hasContextActions = onRename || onDelete;
					if (!hasContextActions) {
						return;
					}
					event.preventDefault(); // Avoid default browser behavior
					event.stopPropagation(); // Avoid trigger parent context menu
					setContextMenu(
						contextMenu
							? undefined
							: {
									path: currentPath,
									clientY: event.clientY,
									clientX: event.clientX,
								},
					);
				}}
				style={
					{
						"--level": parentPath ? parentPath.split("/").length : 0,
					} as CSSProperties
				}
			>
				{isFolder(content) &&
					Object.entries(content)
						.sort(compareFileTreeEntries)
						.map(([filename, child]) =>
							buildTreeItems(filename, filename, child, currentPath),
						)}
			</TreeItem>
		);
	};

	return (
		<SimpleTreeView
			slots={{ collapseIcon: ChevronDownIcon, expandIcon: ChevronRightIcon }}
			aria-label="Files"
			defaultExpandedItems={activePath ? expandablePaths(activePath) : []}
			defaultSelectedItems={activePath}
		>
			{Object.entries(fileTree)
				.sort(compareFileTreeEntries)
				.map(([filename, child]) => buildTreeItems(filename, filename, child))}

			<Menu
				onClose={() => setContextMenu(undefined)}
				open={Boolean(contextMenu)}
				anchorReference="anchorPosition"
				anchorPosition={
					contextMenu
						? {
								top: contextMenu.clientY,
								left: contextMenu.clientX,
							}
						: undefined
				}
				anchorOrigin={{
					vertical: "top",
					horizontal: "left",
				}}
				transformOrigin={{
					vertical: "top",
					horizontal: "left",
				}}
			>
				<MenuItem
					onClick={() => {
						if (!contextMenu) {
							return;
						}
						onRename?.(contextMenu.path);
						setContextMenu(undefined);
					}}
				>
					Rename
				</MenuItem>
				<MenuItem
					onClick={() => {
						if (!contextMenu) {
							return;
						}
						onDelete?.(contextMenu.path);
						setContextMenu(undefined);
					}}
				>
					Delete
				</MenuItem>
			</Menu>
		</SimpleTreeView>
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
