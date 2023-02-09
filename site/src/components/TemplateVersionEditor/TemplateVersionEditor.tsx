import Button from "@material-ui/core/Button"
import IconButton from "@material-ui/core/IconButton"
import { makeStyles, Theme } from "@material-ui/core/styles"
import Tooltip from "@material-ui/core/Tooltip"
import CreateIcon from "@material-ui/icons/AddBox"
import BuildIcon from "@material-ui/icons/BuildOutlined"
import PreviewIcon from "@material-ui/icons/Visibility"
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
import { navHeight } from "theme/constants"
import {
  existsFile,
  getFileContent,
  removeFile,
  setFile,
  TemplateVersionFileTree,
} from "util/templateVersion"
import {
  CreateFileDialog,
  DeleteFileDialog,
  RenameFileDialog,
} from "./FileDialog"
import { FileTree } from "./FileTree"
import { MonacoEditor } from "./MonacoEditor"
import {
  getStatus,
  TemplateVersionStatusBadge,
} from "./TemplateVersionStatusBadge"

interface File {
  path: string
  content?: string
  children: Record<string, File>
}

export interface TemplateVersionEditorProps {
  template: Template
  templateVersion: TemplateVersion
  initialFiles: TemplateVersionFileTree
  buildLogs?: ProvisionerJobLog[]
  resources?: WorkspaceResource[]
  disablePreview: boolean
  disableUpdate: boolean
  onPreview: (files: TemplateVersionFileTree) => void
  onUpdate: () => void
}

const topbarHeight = navHeight

const findInitialFile = (
  fileTree: TemplateVersionFileTree,
  parent?: string,
): File | undefined => {
  for (const key of Object.keys(fileTree)) {
    const currentPath = parent ? `${parent}/${key}` : key

    if (key.endsWith(".tf")) {
      return {
        path: currentPath,
        content: fileTree[key] as string,
        children: {},
      }
    }

    if (typeof fileTree[key] !== "string") {
      return findInitialFile(
        fileTree[key] as TemplateVersionFileTree,
        currentPath,
      )
    }
  }
}

export const TemplateVersionEditor: FC<TemplateVersionEditorProps> = ({
  disablePreview,
  disableUpdate,
  template,
  templateVersion,
  initialFiles,
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
  const [fileTree, setFileTree] = useState(initialFiles)
  const [createFileOpen, setCreateFileOpen] = useState(false)
  const [deleteFileOpen, setDeleteFileOpen] = useState<File>()
  const [renameFileOpen, setRenameFileOpen] = useState<File>()
  const [activeFile, setActiveFile] = useState<File | undefined>(() =>
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
          <div>Used By: {template.active_user_count} developers</div>
        </div>

        <div className={styles.topbarSides}>
          <div className={styles.buildStatus}>
            Build Status:
            <TemplateVersionStatusBadge version={templateVersion} />
          </div>

          <Button
            size="small"
            variant="outlined"
            color="primary"
            disabled={disablePreview}
            onClick={() => {
              triggerPreview()
            }}
          >
            Build (Ctrl + Enter)
          </Button>

          <Tooltip
            title={
              dirty
                ? "You have edited files! Run another build before updating."
                : templateVersion.job.status !== "succeeded"
                ? "Something"
                : ""
            }
          >
            <span>
              <Button
                size="small"
                variant="contained"
                color="primary"
                disabled={dirty || disableUpdate}
                onClick={() => {
                  onUpdate()
                }}
              >
                Publish New Version
              </Button>
            </span>
          </Tooltip>
        </div>
      </div>

      <div className={styles.sidebarAndEditor}>
        <div className={styles.sidebar}>
          <div className={styles.sidebarTitle}>
            Template Editor
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
                setActiveFile({
                  path,
                  content: "",
                  children: {},
                })
                setCreateFileOpen(false)
                setDirty(true)
              }}
            />
            <DeleteFileDialog
              onConfirm={() => {
                if (!deleteFileOpen) {
                  throw new Error("delete file must be set")
                }
                setFileTree((fileTree) =>
                  removeFile(deleteFileOpen.path, fileTree),
                )
                setDeleteFileOpen(undefined)
                if (activeFile?.path === deleteFileOpen.path) {
                  setActiveFile(undefined)
                }
                setDirty(true)
              }}
              open={Boolean(deleteFileOpen)}
              onClose={() => setDeleteFileOpen(undefined)}
              filename={deleteFileOpen?.path || ""}
            />
            <RenameFileDialog
              open={Boolean(renameFileOpen)}
              onClose={() => {
                setRenameFileOpen(undefined)
              }}
              filename={renameFileOpen?.path || ""}
              checkExists={(path) => existsFile(path, fileTree)}
              onConfirm={(newPath) => {
                if (!renameFileOpen) {
                  return
                }
                setFileTree((fileTree) => {
                  fileTree = setFile(
                    newPath,
                    getFileContent(renameFileOpen.path, fileTree),
                    fileTree,
                  )
                  fileTree = removeFile(renameFileOpen.path, fileTree)
                  return fileTree
                })
                renameFileOpen.path = newPath
                setActiveFile(renameFileOpen)
                setRenameFileOpen(undefined)
                setDirty(true)
              }}
            />
          </div>
          <FileTree
            files={fileTree}
            onDelete={(file) => setDeleteFileOpen(file)}
            onSelect={(file) => setActiveFile(file)}
            onRename={(file) => setRenameFileOpen(file)}
            activeFile={activeFile}
          />
        </div>

        <div className={styles.editorPane}>
          <div className={styles.editor} data-chromatic="ignore">
            {activeFile ? (
              <MonacoEditor
                value={activeFile?.content}
                path={activeFile?.path}
                onChange={(value) => {
                  if (!activeFile) {
                    return
                  }
                  setFileTree((fileTree) =>
                    setFile(activeFile.path, value, fileTree),
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
  },
  topbar: {
    padding: theme.spacing(2),
    borderBottom: `1px solid ${theme.palette.divider}`,
    display: "flex",
    alignItems: "center",
    justifyContent: "space-between",
    height: topbarHeight,
  },
  topbarSides: {
    display: "flex",
    alignItems: "center",
    gap: 16,
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
  },
  sidebarTitle: {
    fontSize: 12,
    textTransform: "uppercase",
    padding: "8px 16px",
    color: theme.palette.text.hint,
    display: "flex",
    alignItems: "center",
  },
  sidebarActions: {
    marginLeft: "auto",
    "& svg": {
      fill: theme.palette.text.hint,
    },
  },
  editorPane: {
    display: "grid",
    width: "100%",
    gridTemplateColumns: (props) =>
      props.showBuildLogs ? "0.6fr 0.4fr" : "1fr 0fr",
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
    padding: "8px 12px",
    fontSize: 14,
    background: "transparent",
    fontFamily: "inherit",
    border: 0,
    color: theme.palette.text.hint,
    transition: "150ms ease all",
    display: "flex",
    gap: 8,
    alignItems: "center",
    justifyContent: "center",

    "& svg": {
      maxWidth: 16,
      maxHeight: 16,
    },

    "&.active": {
      color: "white",
      background: theme.palette.background.paperLight,
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
