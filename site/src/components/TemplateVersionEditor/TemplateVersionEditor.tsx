import Button from "@material-ui/core/Button"
import { makeStyles } from "@material-ui/core/styles"
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
import { FC, useEffect, useState } from "react"
import { navHeight } from "theme/constants"
import { TemplateVersionFiles } from "util/templateVersion"
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

  onPreview: (files: TemplateVersionFiles) => void
  onUpdate: () => void
}

const topbarHeight = 90

export const TemplateVersionEditor: FC<TemplateVersionEditorProps> = ({
  template,
  templateVersion,
  initialFiles,
  onPreview,
  onUpdate,
  buildLogs,
  resources,
}) => {
  const styles = useStyles()
  const [files, setFiles] = useState(initialFiles)
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
          onPreview(files)
          break
      }
    }
    document.addEventListener("keydown", keyListener)
    return () => {
      document.removeEventListener("keydown", keyListener)
    }
  }, [files, onPreview])
  const hasIcon = template.icon && template.icon !== ""

  return (
    <div className={styles.root}>
      <div className={styles.topbar}>
        <div>
          Edit Template
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

        <div>
          <Button variant="text">Cancel Changes</Button>
          <Button
            variant="outlined"
            color="primary"
            onClick={() => {
              onPreview(files)
            }}
          >
            Preview (Ctrl + Enter)
          </Button>

          <Button
            variant="contained"
            color="primary"
            disabled={templateVersion.job.status !== "succeeded"}
            onClick={() => {
              onUpdate()
            }}
          >
            Update
          </Button>
        </div>
      </div>

      <div className={styles.sidebarAndEditor}>
        <div className={styles.sidebar}>
          <div className={styles.sidebarTitle}>File Explorer</div>
          <FileTree
            files={files}
            onSelect={(file) => setActiveFile(file)}
            activeFile={activeFile}
          />
        </div>

        <div className={styles.editorPane}>
          <div className={styles.editor}>
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
          </div>

          <div className={styles.panelWrapper}>
            <div className={styles.panel}>
              <div className={styles.tabBar}>Build Logs</div>

              <div className={styles.buildLogs}>
                {buildLogs && <WorkspaceBuildLogs logs={buildLogs} />}
                {templateVersion.job.error && (
                  <div className={styles.buildLogError}>
                    {templateVersion.job.error}
                  </div>
                )}
              </div>
            </div>

            <div className={styles.panelDivider} />

            <div className={styles.panel}>
              <div className={`${styles.tabBar} top`}>Resources</div>
              <div className={styles.resources}>
                {resources && (
                  <TemplateResourcesTable
                    resources={resources.filter(
                      (r) => r.workspace_transition === "start",
                    )}
                  />
                )}
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  )
}

const useStyles = makeStyles((theme) => ({
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
    justifyContent: "space-between",
    height: topbarHeight,
  },
  panelDivider: {
    height: 1,
    background: theme.palette.divider,
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
  },
  panel: {
    height: "50%",
    flex: 1,
    display: "flex",
    flexDirection: "column",
    borderLeft: `1px solid ${theme.palette.divider}`,
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
