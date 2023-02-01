import Button from "@material-ui/core/Button"
import { makeStyles } from "@material-ui/core/styles"
import { ProvisionerJobLog, Template, TemplateVersion, WorkspaceResource } from "api/typesGenerated"
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

  onBuild: (files: TemplateVersionFiles) => void
}

const topbarHeight = 90

export const TemplateVersionEditor: FC<TemplateVersionEditorProps> = ({
  template,
  templateVersion,
  initialFiles,
  onBuild,
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
    const saveListener = (event: KeyboardEvent) => {
      if (
        (navigator.platform.match("Mac") ? event.metaKey : event.ctrlKey) &&
        event.key === "s"
      ) {
        // Prevent opening the save dialog!
        event.preventDefault()
      }
    }
    document.addEventListener("keydown", saveListener)
    return () => {
      document.removeEventListener("keydown", saveListener)
    }
  }, [])
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
        </div>

        <div>
          <Button variant="text">Cancel Changes</Button>
          <Button
            variant="outlined"
            color="primary"
            onClick={() => {
              onBuild(files)
            }}
          >
            Preview
          </Button>

          <Button
            variant="outlined"
            color="primary"
            onClick={() => {
              onBuild(files)
            }}
          >
            Publish
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
          <MonacoEditor value={activeFile?.content} onChange={(value) => {
            setFiles({
              ...files,
              [activeFile?.path || ""]: value,
            })
          }} />
        </div>

        <div className={styles.panel}>
          <div className={styles.buildLogs}>
          {buildLogs && <WorkspaceBuildLogs logs={buildLogs} />}
          {templateVersion.job.error && (
            <div className={styles.buildLogError}>
          {templateVersion.job.error}
            </div>
          )}
          </div>
          <div className={styles.resources}>
          {resources && <TemplateResourcesTable resources={resources} />}
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
  panel: {
    flex: 1,
    display: "flex",
    flexDirection: "column",
  },
  buildLogs: {
    height: "50%",
    display: "flex",
    flexDirection: "column-reverse",
    overflowY: "auto",
  },
  buildLogError: {
    whiteSpace: "pre-wrap",
  },
  resources: {
    height: "50%",
    overflowY: "auto",
  },
}))
