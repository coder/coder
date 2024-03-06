import { css } from "@emotion/react";
import ChevronRightIcon from "@mui/icons-material/ChevronRight";
import ExpandMoreIcon from "@mui/icons-material/ExpandMore";
import FormatAlignLeftOutlined from "@mui/icons-material/FormatAlignLeftOutlined";
import TreeItem from "@mui/lab/TreeItem";
import TreeView from "@mui/lab/TreeView";
import Menu from "@mui/material/Menu";
import MenuItem from "@mui/material/MenuItem";
import { type CSSProperties, type FC, useState } from "react";
import { DockerIcon } from "components/Icons/DockerIcon";
import type { FileTree } from "utils/filetree";

const sortFileTree = (fileTree: FileTree) => (a: string, b: string) => {
  const contentA = fileTree[a];
  const contentB = fileTree[b];
  if (typeof contentA === "object") {
    return -1;
  }
  if (typeof contentB === "object") {
    return 1;
  }
  return a.localeCompare(b);
};

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

  const isFolder = (content?: FileTree | string): content is FileTree =>
    typeof content === "object";

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

    let icon: JSX.Element | null = isFolder(content) ? null : (
      <FormatAlignLeftOutlined />
    );

    if (filename.endsWith(".tf")) {
      icon = <FileTypeTerraform />;
    }
    if (filename.endsWith(".md")) {
      icon = <FileTypeMarkdown />;
    }
    if (filename.endsWith("Dockerfile")) {
      icon = <DockerIcon />;
    }

    return (
      <TreeItem
        nodeId={currentPath}
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
            padding: 2px 16px;
            color: ${isHiddenFile
              ? theme.palette.text.disabled
              : theme.palette.text.secondary};
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
        icon={icon}
        style={
          {
            "--level": parentPath ? parentPath.split("/").length : 0,
          } as CSSProperties
        }
      >
        {isFolder(content) &&
          Object.keys(content)
            .sort(sortFileTree(content))
            .map((filename) => {
              const child = content[filename];
              return buildTreeItems(filename, filename, child, currentPath);
            })}
      </TreeItem>
    );
  };

  return (
    <TreeView
      defaultCollapseIcon={<ExpandMoreIcon />}
      defaultExpandIcon={<ChevronRightIcon />}
      aria-label="Files"
      defaultExpanded={activePath ? expandablePaths(activePath) : []}
      defaultSelected={activePath}
    >
      {Object.keys(fileTree)
        .sort(sortFileTree(fileTree))
        .map((filename) => {
          const child = fileTree[filename];
          return buildTreeItems(filename, filename, child);
        })}

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
            onRename && onRename(contextMenu.path);
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
            onDelete && onDelete(contextMenu.path);
            setContextMenu(undefined);
          }}
        >
          Delete
        </MenuItem>
      </Menu>
    </TreeView>
  );
};

const FileTypeTerraform: FC = () => (
  <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 32" fill="#813cf3">
    <title>file_type_terraform</title>
    <polygon points="12.042 6.858 20.071 11.448 20.071 20.462 12.042 15.868 12.042 6.858 12.042 6.858" />
    <polygon points="20.5 20.415 28.459 15.84 28.459 6.887 20.5 11.429 20.5 20.415 20.5 20.415" />
    <polygon points="3.541 11.01 11.571 15.599 11.571 6.59 3.541 2 3.541 11.01 3.541 11.01" />
    <polygon points="12.042 25.41 20.071 30 20.071 20.957 12.042 16.368 12.042 25.41 12.042 25.41" />
  </svg>
);

const FileTypeMarkdown: FC = () => (
  <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 32" fill="#755838">
    <rect
      x="2.5"
      y="7.955"
      width="27"
      height="16.091"
      style={{
        fill: "none",
        stroke: "#755838",
      }}
    />
    <polygon points="5.909 20.636 5.909 11.364 8.636 11.364 11.364 14.773 14.091 11.364 16.818 11.364 16.818 20.636 14.091 20.636 14.091 15.318 11.364 18.727 8.636 15.318 8.636 20.636 5.909 20.636" />
    <polygon points="22.955 20.636 18.864 16.136 21.591 16.136 21.591 11.364 24.318 11.364 24.318 16.136 27.045 16.136 22.955 20.636" />
  </svg>
);

const expandablePaths = (path: string) => {
  const paths = path.split("/");
  const result = [];
  for (let i = 1; i < paths.length; i++) {
    result.push(paths.slice(0, i).join("/"));
  }
  return result;
};
