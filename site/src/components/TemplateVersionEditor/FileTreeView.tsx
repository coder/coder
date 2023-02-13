import { makeStyles } from "@material-ui/core/styles"
import ChevronRightIcon from "@material-ui/icons/ChevronRight"
import ExpandMoreIcon from "@material-ui/icons/ExpandMore"
import TreeView from "@material-ui/lab/TreeView"
import TreeItem from "@material-ui/lab/TreeItem"
import Menu from "@material-ui/core/Menu"
import MenuItem from "@material-ui/core/MenuItem"
import { FC, useState } from "react"
import { FileTree } from "util/filetree"
import { DockerIcon } from "components/Icons/DockerIcon"

const sortFileTree = (fileTree: FileTree) => (a: string, b: string) => {
  const contentA = fileTree[a]
  const contentB = fileTree[b]
  if (typeof contentA === "object") {
    return -1
  }
  if (typeof contentB === "object") {
    return 1
  }
  return a.localeCompare(b)
}

type ContextMenu = {
  path: string
  clientX: number
  clientY: number
}

export const FileTreeView: FC<{
  onSelect: (path: string) => void
  onDelete: (path: string) => void
  onRename: (path: string) => void
  fileTree: FileTree
  activePath?: string
}> = ({ fileTree, activePath, onDelete, onRename, onSelect }) => {
  const styles = useStyles()
  const [contextMenu, setContextMenu] = useState<ContextMenu | undefined>()

  const buildTreeItems = (
    filename: string,
    content?: FileTree | string,
    parentPath?: string,
  ): JSX.Element => {
    const currentPath = parentPath ? `${parentPath}/${filename}` : filename
    let icon: JSX.Element | null = null
    if (filename.endsWith(".tf")) {
      icon = <FileTypeTerraform />
    }
    if (filename.endsWith(".md")) {
      icon = <FileTypeMarkdown />
    }
    if (filename.endsWith("Dockerfile")) {
      icon = <FileTypeDockerfile />
    }

    return (
      <TreeItem
        nodeId={currentPath}
        key={currentPath}
        label={filename}
        className={`${styles.fileTreeItem} ${
          currentPath === activePath ? "active" : ""
        }`}
        onClick={() => {
          onSelect(currentPath)
        }}
        onContextMenu={(event) => {
          event.preventDefault()
          setContextMenu(
            contextMenu
              ? undefined
              : {
                  path: currentPath,
                  clientY: event.clientY,
                  clientX: event.clientX,
                },
          )
        }}
        icon={icon}
      >
        {typeof content === "object" ? (
          Object.keys(content)
            .sort(sortFileTree(content))
            .map((filename) => {
              const child = content[filename]
              return buildTreeItems(filename, child, currentPath)
            })
        ) : (
          <></>
        )}
      </TreeItem>
    )
  }

  return (
    <TreeView
      defaultCollapseIcon={<ExpandMoreIcon />}
      defaultExpandIcon={<ChevronRightIcon />}
      aria-label="Files"
      className={styles.fileTree}
    >
      {Object.keys(fileTree)
        .sort(sortFileTree(fileTree))
        .map((filename) => {
          const child = fileTree[filename]
          return buildTreeItems(filename, child)
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
              return
            }
            onRename(contextMenu.path)
            setContextMenu(undefined)
          }}
        >
          Rename...
        </MenuItem>
        <MenuItem
          onClick={() => {
            if (!contextMenu) {
              return
            }
            onDelete(contextMenu.path)
            setContextMenu(undefined)
          }}
        >
          Delete Permanently
        </MenuItem>
      </Menu>
    </TreeView>
  )
}

const useStyles = makeStyles((theme) => ({
  fileTree: {},
  fileTreeItem: {
    overflow: "hidden",
    userSelect: "none",

    "&:focus": {
      "& > .MuiTreeItem-content": {
        background: "rgba(255, 255, 255, 0.1)",
      },
    },
    "& > .MuiTreeItem-content:hover": {
      background: theme.palette.background.paperLight,
      color: theme.palette.text.primary,
    },

    "& > .MuiTreeItem-content": {
      padding: "1px 16px",
      color: theme.palette.text.secondary,

      "& svg": {
        width: 16,
        height: 16,
      },

      "& > .MuiTreeItem-label": {
        marginLeft: 4,
        fontSize: 14,
        color: "inherit",
      },
    },

    "&.active": {
      background: theme.palette.background.paper,

      "& > .MuiTreeItem-content": {
        color: theme.palette.text.primary,
      },
    },
  },
  editor: {
    flex: 1,
  },
  preview: {},
}))

const FileTypeTerraform = () => (
  <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 32" fill="#813cf3">
    <title>file_type_terraform</title>
    <polygon points="12.042 6.858 20.071 11.448 20.071 20.462 12.042 15.868 12.042 6.858 12.042 6.858" />
    <polygon points="20.5 20.415 28.459 15.84 28.459 6.887 20.5 11.429 20.5 20.415 20.5 20.415" />
    <polygon points="3.541 11.01 11.571 15.599 11.571 6.59 3.541 2 3.541 11.01 3.541 11.01" />
    <polygon points="12.042 25.41 20.071 30 20.071 20.957 12.042 16.368 12.042 25.41 12.042 25.41" />
  </svg>
)

const FileTypeMarkdown = () => (
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
)

const FileTypeDockerfile = () => <DockerIcon />
