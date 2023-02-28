import Button from "@material-ui/core/Button"
import IconButton from "@material-ui/core/IconButton"
import { makeStyles, Theme } from "@material-ui/core/styles"
import Tooltip from "@material-ui/core/Tooltip"
import CreateIcon from "@material-ui/icons/AddOutlined"
import BuildIcon from "@material-ui/icons/BuildOutlined"
import PreviewIcon from "@material-ui/icons/VisibilityOutlined"
import {
  ProvisionerJobLog,
  Template,
  TemplateVersion,
  WorkspaceResource,
} from "api/typesGenerated"
import { Avatar } from "components/Avatar/Avatar"
import { AvatarData } from "components/AvatarData/AvatarData"
import { TemplateResourcesTable } from "components/TemplateResourcesTable/TemplateResourcesTable"
import { WorkspaceBuildLogs } from "components/WorkspaceBuildLogs/WorkspaceBuildLogs"
import { FC, useCallback, useEffect, useRef, useState } from "react"
import { navHeight, dashboardContentBottomPadding } from "theme/constants"
import {
  existsFile,
  FileTree,
  getFileContent,
  isFolder,
  removeFile,
  setFile,
  traverse,
} from "util/filetree"
import {
  CreateFileDialog,
  DeleteFileDialog,
  RenameFileDialog,
} from "./FileDialog"
import { FileTreeView } from "./FileTreeView"
import { MonacoEditor } from "./MonacoEditor"
import {
  getStatus,
  TemplateVersionStatusBadge,
} from "./TemplateVersionStatusBadge"

export interface TemplateVersionEditorProps {
  template: Template
  templateVersion: TemplateVersion
  defaultFileTree: FileTree
  buildLogs?: ProvisionerJobLog[]
  resources?: WorkspaceResource[]
  disablePreview: boolean
  disableUpdate: boolean
  onPreview: (files: FileTree) => void
  onUpdate: () => void
}

const topbarHeight = 80

const findInitialFile = (fileTree: FileTree): string | undefined => {
  let initialFile: string | undefined

  traverse(fileTree, (content, filename, path) => {
    if (filename.endsWith(".tf")) {
      initialFile = path
    }
  })

  return initialFile
}

export const TemplateVersionEditor: FC<TemplateVersionEditorProps> = ({
  disablePreview,
  disableUpdate,
  template,
  templateVersion,
  defaultFileTree,
  onPreview,
  onUpdate,
  buildLogs,
  resources,
}) => {
  const [selectedTab, setSelectedTab] = useState(() => {
    // If resources are provided, show them by default!
    // This is for Storybook!
    return resources ? 1 : 0
  })
  const [fileTree, setFileTree] = useState(defaultFileTree)
  const [createFileOpen, setCreateFileOpen] = useState(false)
  const [deleteFileOpen, setDeleteFileOpen] = useState<string>()
  const [renameFileOpen, setRenameFileOpen] = useState<string>()
  const [activePath, setActivePath] = useState<string | undefined>(() =>
    findInitialFile(fileTree),
  )

  const triggerPreview = useCallback(() => {
    onPreview(fileTree)
    // Switch to the build log!
    setSelectedTab(0)
  }, [fileTree, onPreview])

  // Stop ctrl+s from saving files and make ctrl+enter trigger a preview.
  useEffect(() => {
    const keyListener = (event: KeyboardEvent) => {
      if (!(navigator.platform.match("Mac") ? event.metaKey : event.ctrlKey)) {
        return
      }
      switch (event.key) {
        case "s":
          // Prevent opening the save dialog!
          event.preventDefault()
          break
        case "Enter":
          event.preventDefault()
          triggerPreview()
          break
      }
    }
    document.addEventListener("keydown", keyListener)
    return () => {
      document.removeEventListener("keydown", keyListener)
    }
  }, [triggerPreview])

  // Automatically switch to the template preview tab when the build succeeds.
  const previousVersion = useRef<TemplateVersion>()
  useEffect(() => {
    if (!previousVersion.current) {
      previousVersion.current = templateVersion
      return
    }
    if (
      previousVersion.current.job.status === "running" &&
      templateVersion.job.status === "succeeded"
    ) {
      setSelectedTab(1)
      setDirty(false)
    }
    previousVersion.current = templateVersion
  }, [templateVersion])

  const [dirty, setDirty] = useState(false)
  const hasIcon = template.icon && template.icon !== ""
  const templateVersionSucceeded = templateVersion.job.status === "succeeded"
  const showBuildLogs = Boolean(buildLogs)
  const editorValue = getFileContent(activePath ?? "", fileTree) as string

  useEffect(() => {
    window.dispatchEvent(new Event("resize"))
  }, [showBuildLogs])
  const styles = useStyles({
    templateVersionSucceeded,
    showBuildLogs,
  })

  return (
    <div className={styles.root}>
      <div className={styles.topbar}>
        <div className={styles.topbarSides}>
          <AvatarData
            title={template.display_name || template.name}
            subtitle={template.description}
            avatar={
              hasIcon && (
                <Avatar src={template.icon} variant="square" fitImage />
              )
            }
          />
        </div>

        <div className={styles.topbarSides}>
          <div className={styles.buildStatus}>
            <TemplateVersionStatusBadge version={templateVersion} />
          </div>

          <Button
            title="Build template (Ctrl + Enter)"
            size="small"
            variant="outlined"
            disabled={disablePreview}
            onClick={() => {
              triggerPreview()
            }}
          >
            Build template
          </Button>

          <Button
            title={
              dirty
                ? "You have edited files! Run another build before updating."
                : templateVersion.job.status !== "succeeded"
                ? "Something"
                : ""
            }
            size="small"
            disabled={dirty || disableUpdate}
            onClick={onUpdate}
          >
            Publish version
          </Button>
        </div>
      </div>

      <div className={styles.sidebarAndEditor}>
        <div className={styles.sidebar}>
          <div className={styles.sidebarTitle}>
            Template files
            <div className={styles.sidebarActions}>
              <Tooltip title="Create File" placement="top">
                <IconButton
                  size="small"
                  aria-label="Create File"
                  onClick={(event) => {
                    setCreateFileOpen(true)
                    event.currentTarget.blur()
                  }}
                >
                  <CreateIcon />
                </IconButton>
              </Tooltip>
            </div>
            <CreateFileDialog
              open={createFileOpen}
              onClose={() => {
                setCreateFileOpen(false)
              }}
              checkExists={(path) => existsFile(path, fileTree)}
              onConfirm={(path) => {
                setFileTree((fileTree) => setFile(path, "", fileTree))
                setActivePath(path)
                setCreateFileOpen(false)
                setDirty(true)
              }}
            />
            <DeleteFileDialog
              onConfirm={() => {
                if (!deleteFileOpen) {
                  throw new Error("delete file must be set")
                }
                setFileTree((fileTree) => removeFile(deleteFileOpen, fileTree))
                setDeleteFileOpen(undefined)
                if (activePath === deleteFileOpen) {
                  setActivePath(undefined)
                }
                setDirty(true)
              }}
              open={Boolean(deleteFileOpen)}
              onClose={() => setDeleteFileOpen(undefined)}
              filename={deleteFileOpen || ""}
            />
            <RenameFileDialog
              open={Boolean(renameFileOpen)}
              onClose={() => {
                setRenameFileOpen(undefined)
              }}
              filename={renameFileOpen || ""}
              checkExists={(path) => existsFile(path, fileTree)}
              onConfirm={(newPath) => {
                if (!renameFileOpen) {
                  return
                }
                setFileTree((fileTree) => {
                  fileTree = setFile(
                    newPath,
                    getFileContent(renameFileOpen, fileTree) as string,
                    fileTree,
                  )
                  fileTree = removeFile(renameFileOpen, fileTree)
                  return fileTree
                })
                setActivePath(newPath)
                setRenameFileOpen(undefined)
                setDirty(true)
              }}
            />
          </div>
          <FileTreeView
            fileTree={fileTree}
            onDelete={(file) => setDeleteFileOpen(file)}
            onSelect={(filePath) => {
              if (!isFolder(filePath, fileTree)) {
                setActivePath(filePath)
              }
            }}
            onRename={(file) => setRenameFileOpen(file)}
            activePath={activePath}
          />
        </div>

        <div className={styles.editorPane}>
          <div className={styles.editor} data-chromatic="ignore">
            {activePath ? (
              <MonacoEditor
                value={editorValue}
                path={activePath}
                onChange={(value) => {
                  if (!activePath) {
                    return
                  }
                  setFileTree((fileTree) =>
                    setFile(activePath, value, fileTree),
                  )
                  setDirty(true)
                }}
              />
            ) : (
              <div>No file opened</div>
            )}
          </div>

          <div className={styles.panelWrapper}>
            <div className={styles.tabs}>
              <button
                className={`${styles.tab} ${selectedTab === 0 ? "active" : ""}`}
                onClick={() => {
                  setSelectedTab(0)
                }}
              >
                {templateVersion.job.status !== "succeeded" ? (
                  getStatus(templateVersion).icon
                ) : (
                  <BuildIcon />
                )}
                Build Log
              </button>

              {!disableUpdate && (
                <button
                  className={`${styles.tab} ${
                    selectedTab === 1 ? "active" : ""
                  }`}
                  onClick={() => {
                    setSelectedTab(1)
                  }}
                >
                  <PreviewIcon />
                  Workspace Preview
                </button>
              )}
            </div>

            <div
              className={`${styles.panel} ${styles.buildLogs} ${
                selectedTab === 0 ? "" : "hidden"
              }`}
            >
              {buildLogs && (
                <WorkspaceBuildLogs
                  templateEditorPane
                  hideTimestamps
                  logs={buildLogs}
                />
              )}
              {templateVersion.job.error && (
                <div className={styles.buildLogError}>
                  {templateVersion.job.error}
                </div>
              )}
            </div>

            <div
              className={`${styles.panel} ${styles.resources} ${
                selectedTab === 1 ? "" : "hidden"
              }`}
            >
              {resources && (
                <TemplateResourcesTable
                  resources={resources.filter(
                    (r) => r.workspace_transition === "start",
                  )}
                />
              )}
            </div>
          </div>

          {templateVersionSucceeded && (
            <>
              <div className={styles.panelDivider} />
            </>
          )}
        </div>
      </div>
    </div>
  )
}

const useStyles = makeStyles<
  Theme,
  {
    templateVersionSucceeded: boolean
    showBuildLogs: boolean
  }
>((theme) => ({
  root: {
    height: `calc(100vh - ${navHeight}px)`,
    background: theme.palette.background.default,
    flex: 1,
    display: "flex",
    flexDirection: "column",
    marginBottom: -dashboardContentBottomPadding, // Remove dashboard bottom padding
  },
  topbar: {
    padding: theme.spacing(2),
    borderBottom: `1px solid ${theme.palette.divider}`,
    display: "flex",
    alignItems: "center",
    justifyContent: "space-between",
    height: topbarHeight,
    background: theme.palette.background.paper,
  },
  topbarSides: {
    display: "flex",
    alignItems: "center",
    gap: theme.spacing(2),
  },
  buildStatus: {
    display: "flex",
    alignItems: "center",
    gap: 8,
  },
  sidebarAndEditor: {
    display: "flex",
    flex: 1,
  },
  sidebar: {
    minWidth: 256,
    backgroundColor: theme.palette.background.paper,
    borderRight: `1px solid ${theme.palette.divider}`,
  },
  sidebarTitle: {
    fontSize: 10,
    textTransform: "uppercase",
    padding: theme.spacing(1, 2),
    color: theme.palette.text.primary,
    fontWeight: 500,
    letterSpacing: "0.5px",
    display: "flex",
    alignItems: "center",
  },
  sidebarActions: {
    marginLeft: "auto",
    "& svg": {
      fill: theme.palette.text.primary,
    },
  },
  editorPane: {
    display: "grid",
    width: "100%",
    gridTemplateColumns: (props) =>
      props.showBuildLogs ? "1fr 1fr" : "1fr 0fr",
    height: `calc(100vh - ${navHeight + topbarHeight}px)`,
    overflow: "hidden",
  },
  editor: {
    flex: 1,
  },
  panelWrapper: {
    flex: 1,
    display: "flex",
    flexDirection: "column",
    borderLeft: `1px solid ${theme.palette.divider}`,
    overflowY: "auto",
  },
  panel: {
    padding: theme.spacing(1),

    "&.hidden": {
      display: "none",
    },
  },
  tabs: {
    borderBottom: `1px solid ${theme.palette.divider}`,
    display: "flex",
    boxShadow: "#000000 0 6px 6px -6px inset",

    "& .MuiTab-root": {
      padding: 0,
      fontSize: 14,
      textTransform: "none",
      letterSpacing: "unset",
    },
  },
  tab: {
    cursor: "pointer",
    padding: theme.spacing(1.5),
    fontSize: 10,
    textTransform: "uppercase",
    letterSpacing: "0.5px",
    fontWeight: 600,
    background: "transparent",
    fontFamily: "inherit",
    border: 0,
    color: theme.palette.text.secondary,
    transition: "150ms ease all",
    display: "flex",
    gap: 8,
    alignItems: "center",
    justifyContent: "center",
    position: "relative",

    "& svg": {
      maxWidth: 12,
      maxHeight: 12,
    },

    "&.active": {
      color: theme.palette.text.primary,
      "&:after": {
        content: '""',
        display: "block",
        width: "100%",
        height: 1,
        backgroundColor: theme.palette.text.primary,
        bottom: -1,
        position: "absolute",
      },
    },

    "&:hover": {
      color: theme.palette.text.primary,
    },
  },
  tabBar: {
    padding: "8px 16px",
    position: "sticky",
    top: 0,
    background: theme.palette.background.default,
    borderBottom: `1px solid ${theme.palette.divider}`,
    color: theme.palette.text.hint,
    textTransform: "uppercase",
    fontSize: 12,

    "&.top": {
      borderTop: `1px solid ${theme.palette.divider}`,
    },
  },
  buildLogs: {
    display: "flex",
    flexDirection: "column-reverse",
    overflowY: "auto",
  },
  buildLogError: {
    whiteSpace: "pre-wrap",
  },
  resources: {
    // padding: 16,
  },
}))
