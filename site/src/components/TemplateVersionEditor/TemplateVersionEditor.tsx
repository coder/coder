import Button from "@material-ui/core/Button"
import IconButton from "@material-ui/core/IconButton"
import { makeStyles, Theme } from "@material-ui/core/styles"
import Tab from "@material-ui/core/Tab"
import Tabs from "@material-ui/core/Tabs"
import Tooltip from "@material-ui/core/Tooltip"
import CreateIcon from "@material-ui/icons/AddBox"
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
import { FC, useCallback, useEffect, useState } from "react"
import { navHeight } from "theme/constants"
import { TemplateVersionFiles } from "util/templateVersion"
import {
  CreateFileDialog,
  DeleteFileDialog,
  RenameFileDialog,
} from "./FileDialog"
import { FileTree } from "./FileTree"
import { MonacoEditor } from "./MonacoEditor"

interface File {
  path: string
  content?: string
  children: Record<string, File>
}

export interface TemplateVersionEditorProps {
  template: Template
  templateVersion: TemplateVersion
  initialFiles: TemplateVersionFiles

  buildLogs?: ProvisionerJobLog[]
  resources?: WorkspaceResource[]

  disablePreview: boolean
  disableUpdate: boolean

  onPreview: (files: TemplateVersionFiles) => void
  onUpdate: () => void
}

const topbarHeight = navHeight

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
  const [selectedTab, setSelectedTab] = useState(0)
  const [files, setFiles] = useState(initialFiles)
  const [createFileOpen, setCreateFileOpen] = useState(false)
  const [deleteFileOpen, setDeleteFileOpen] = useState<File>()
  const [renameFileOpen, setRenameFileOpen] = useState<File>()
  const [activeFile, setActiveFile] = useState<File | undefined>(() => {
    const fileKeys = Object.keys(initialFiles)
    for (let i = 0; i < fileKeys.length; i++) {
      if (fileKeys[i].endsWith(".tf")) {
        return {
          path: fileKeys[i],
          content: initialFiles[fileKeys[i]],
          children: {},
        }
      }
    }
  })
  const triggerPreview = useCallback(() => {
    onPreview(files)
    // Switch to the build log!
    setSelectedTab(0)
  }, [files, onPreview])
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
  }, [files, triggerPreview])
  const hasIcon = template.icon && template.icon !== ""
  const templateVersionSucceeded = templateVersion.job.status === "succeeded"
  const styles = useStyles({
    templateVersionSucceeded,
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
              templateVersion.job.status !== "succeeded" ? "Something" : ""
            }
          >
            <Button
              size="small"
              variant="contained"
              color="primary"
              disabled={disableUpdate}
              onClick={() => {
                onUpdate()
              }}
            >
              Update
            </Button>
          </Tooltip>
        </div>
      </div>

      <div className={styles.sidebarAndEditor}>
        <div className={styles.sidebar}>
          <div className={styles.sidebarTitle}>
            Template Editor
            <Tooltip title="Create File" placement="top">
              <IconButton
                size="small"
                color="secondary"
                aria-label="Create File"
                onClick={(event) => {
                  setCreateFileOpen(true)
                  event.currentTarget.blur()
                }}
              >
                <CreateIcon />
              </IconButton>
            </Tooltip>
            <CreateFileDialog
              open={createFileOpen}
              onClose={() => {
                setCreateFileOpen(false)
              }}
              checkExists={(path) => Boolean(files[path])}
              onConfirm={(path) => {
                setFiles({
                  ...files,
                  [path]: "",
                })
                setActiveFile({
                  path,
                  content: "",
                  children: {},
                })
                setCreateFileOpen(false)
              }}
            />
            <DeleteFileDialog
              onConfirm={() => {
                if (!deleteFileOpen) {
                  throw new Error("delete file must be set")
                }
                const deleted = { ...files }
                delete deleted[deleteFileOpen.path]
                setFiles(deleted)
                setDeleteFileOpen(undefined)
                if (activeFile?.path === deleteFileOpen.path) {
                  setActiveFile(undefined)
                }
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
              checkExists={(path) => Boolean(files[path])}
              onConfirm={(newPath) => {
                if (!renameFileOpen) {
                  return
                }
                const renamed = { ...files }
                renamed[newPath] = renamed[renameFileOpen.path]
                delete renamed[renameFileOpen.path]
                setFiles(renamed)
                renameFileOpen.path = newPath
                setActiveFile(renameFileOpen)
                setRenameFileOpen(undefined)
              }}
            />
          </div>
          <FileTree
            files={files}
            onDelete={(file) => setDeleteFileOpen(file)}
            onSelect={(file) => setActiveFile(file)}
            onRename={(file) => setRenameFileOpen(file)}
            activeFile={activeFile}
          />
        </div>

        <div className={styles.editorPane}>
          <div className={styles.editor}>
            {activeFile ? (
              <MonacoEditor
                value={activeFile?.content}
                path={activeFile?.path}
                onChange={(value) => {
                  if (!activeFile) {
                    return
                  }
                  setFiles({
                    ...files,
                    [activeFile.path]: value,
                  })
                }}
              />
            ) : (
              <div>No file opened</div>
            )}
          </div>

          <div className={styles.panelWrapper}>
            <Tabs
              value={selectedTab}
              onChange={(_, value) => {
                setSelectedTab(value)
              }}
              className={styles.tabs}
            >
              <Tab
                style={{ minWidth: 120 }}
                label={<div>Build Log {templateVersion.job.status}</div>}
              />
              {!disableUpdate && (
                <Tab style={{ minWidth: 120 }} label="Preview" />
              )}
            </Tabs>

            <div
              className={`${styles.panel} ${styles.buildLogs} ${
                selectedTab === 0 ? "" : "hidden"
              }`}
            >
              {buildLogs && (
                <WorkspaceBuildLogs hideTimestamps logs={buildLogs} />
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
  },
  editorPane: {
    display: "flex",
    flexDirection: "row",
    flex: 1,
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

    "& .MuiTab-root": {
      padding: 0,
      fontSize: 14,
      textTransform: "none",
      letterSpacing: "unset",
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
    padding: 16,
    overflowY: "auto",
  },
  buildLogError: {
    whiteSpace: "pre-wrap",
  },
  resources: {
    padding: 16,
  },
}))
