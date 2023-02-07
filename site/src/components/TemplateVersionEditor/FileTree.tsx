import { makeStyles } from "@material-ui/core/styles"
import ChevronRightIcon from "@material-ui/icons/ChevronRight"
import ExpandMoreIcon from "@material-ui/icons/ExpandMore"
import TreeView from "@material-ui/lab/TreeView"
import TreeItem from "@material-ui/lab/TreeItem"
import Menu from "@material-ui/core/Menu"
import MenuItem from "@material-ui/core/MenuItem"
import { FC, useMemo, useState } from "react"
import { TemplateVersionFiles } from "util/templateVersion"
import { DockerIcon } from "components/Icons/DockerIcon"

export interface File {
  path: string
  content?: string
  children: Record<string, File>
}

export const FileTree: FC<{
  onSelect: (file: File) => void
  onDelete: (file: File) => void
  onRename: (file: File) => void
  files: TemplateVersionFiles
  activeFile?: File
}> = ({ activeFile, files, onDelete, onRename, onSelect }) => {
  const styles = useStyles()
  const fileTree = useMemo<Record<string, File>>(() => {
    const paths = Object.keys(files)
    const roots: Record<string, File> = {}
    paths.forEach((path) => {
      const pathParts = path.split("/")
      const firstPart = pathParts.shift()
      if (!firstPart) {
        // Not possible!
        return
      }
      let activeFile = roots[firstPart]
      if (!activeFile) {
        activeFile = {
          path: firstPart,
          children: {},
        }
        roots[firstPart] = activeFile
      }
      while (pathParts.length > 0) {
        const pathPart = pathParts.shift()
        if (!pathPart) {
          continue
        }
        if (!activeFile.children[pathPart]) {
          activeFile.children[pathPart] = {
            path: activeFile.path + "/" + pathPart,
            children: {},
          }
        }
        activeFile = activeFile.children[pathPart]
      }
      activeFile.content = files[path]
      activeFile.path = path
    })
    return roots
  }, [files])
  const [contextMenu, setContextMenu] = useState<
    | {
        file: File
        clientX: number
        clientY: number
      }
    | undefined
  >()

  const buildTreeItems = (name: string, file: File): JSX.Element => {
    let icon: JSX.Element | null = null
    if (file.path.endsWith(".tf")) {
      icon = <FileTypeTerraform />
    }
    if (file.path.endsWith(".md")) {
      icon = <FileTypeMarkdown />
    }
    if (file.path.endsWith("Dockerfile")) {
      icon = <FileTypeDockerfile />
    }

    return (
      <TreeItem
        nodeId={file.path}
        key={file.path}
        label={name}
        className={`${styles.fileTreeItem} ${
          file.path === activeFile?.path ? "active" : ""
        }`}
        onClick={() => {
          if (file.content) {
            onSelect(file)
          }
        }}
        onContextMenu={(event) => {
          event.preventDefault()
          if (!file.content) {
            return
          }
          setContextMenu(
            contextMenu
              ? undefined
              : {
                  file: file,
                  clientY: event.clientY,
                  clientX: event.clientX,
                },
          )
        }}
        icon={icon}
      >
        {Object.entries(file.children || {}).map(([name, file]) => {
          return buildTreeItems(name, file)
        })}
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
      {Object.entries(fileTree).map(([name, file]) => {
        return buildTreeItems(name, file)
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
            onRename(contextMenu.file)
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
            onDelete(contextMenu.file)
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
