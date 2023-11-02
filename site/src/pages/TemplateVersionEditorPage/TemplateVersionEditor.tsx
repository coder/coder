import Button from "@mui/material/Button";
import IconButton from "@mui/material/IconButton";
import Link from "@mui/material/Link";
import Tooltip from "@mui/material/Tooltip";
import CreateIcon from "@mui/icons-material/AddOutlined";
import BuildIcon from "@mui/icons-material/BuildOutlined";
import PreviewIcon from "@mui/icons-material/VisibilityOutlined";
import {
  ProvisionerJobLog,
  Template,
  TemplateVersion,
  TemplateVersionVariable,
  VariableValue,
  WorkspaceResource,
} from "api/typesGenerated";
import { Link as RouterLink } from "react-router-dom";
import { Alert, AlertDetail } from "components/Alert/Alert";
import { Avatar } from "components/Avatar/Avatar";
import { AvatarData } from "components/AvatarData/AvatarData";
import { TemplateResourcesTable } from "components/TemplateResourcesTable/TemplateResourcesTable";
import { WorkspaceBuildLogs } from "components/WorkspaceBuildLogs/WorkspaceBuildLogs";
import { PublishVersionData } from "pages/TemplateVersionEditorPage/types";
import { type FC, useCallback, useEffect, useRef, useState } from "react";
import {
  createFile,
  existsFile,
  FileTree,
  getFileContent,
  isFolder,
  moveFile,
  removeFile,
  traverse,
  updateFile,
} from "utils/filetree";
import {
  CreateFileDialog,
  DeleteFileDialog,
  RenameFileDialog,
} from "./FileDialog";
import { FileTreeView } from "./FileTreeView";
import { MissingTemplateVariablesDialog } from "./MissingTemplateVariablesDialog";
import { MonacoEditor } from "./MonacoEditor";
import { PublishTemplateVersionDialog } from "./PublishTemplateVersionDialog";
import {
  getStatus,
  TemplateVersionStatusBadge,
} from "./TemplateVersionStatusBadge";
import AlertTitle from "@mui/material/AlertTitle";
import { DashboardFullPage } from "components/Dashboard/DashboardLayout";
import { type Interpolation, type Theme, useTheme } from "@emotion/react";

type Tab = "logs" | "resources" | undefined; // Undefined is to hide the tab
export interface TemplateVersionEditorProps {
  template: Template;
  templateVersion: TemplateVersion;
  defaultFileTree: FileTree;
  buildLogs?: ProvisionerJobLog[];
  resources?: WorkspaceResource[];
  disablePreview: boolean;
  disableUpdate: boolean;
  onPreview: (files: FileTree) => void;
  onPublish: () => void;
  onConfirmPublish: (data: PublishVersionData) => void;
  onCancelPublish: () => void;
  publishingError: unknown;
  publishedVersion?: TemplateVersion;
  onCreateWorkspace: () => void;
  isAskingPublishParameters: boolean;
  isPromptingMissingVariables: boolean;
  isPublishing: boolean;
  missingVariables?: TemplateVersionVariable[];
  onSubmitMissingVariableValues: (values: VariableValue[]) => void;
  onCancelSubmitMissingVariableValues: () => void;
  defaultTab?: Tab;
}

const topbarHeight = 80;

const findInitialFile = (fileTree: FileTree): string | undefined => {
  let initialFile: string | undefined;

  traverse(fileTree, (content, filename, path) => {
    if (filename.endsWith(".tf")) {
      initialFile = path;
    }
  });

  return initialFile;
};

export const TemplateVersionEditor: FC<TemplateVersionEditorProps> = ({
  disablePreview,
  disableUpdate,
  template,
  templateVersion,
  defaultFileTree,
  onPreview,
  onPublish,
  onConfirmPublish,
  onCancelPublish,
  isAskingPublishParameters,
  isPublishing,
  publishingError,
  publishedVersion,
  onCreateWorkspace,
  buildLogs,
  resources,
  isPromptingMissingVariables,
  missingVariables,
  onSubmitMissingVariableValues,
  onCancelSubmitMissingVariableValues,
  defaultTab,
}) => {
  const theme = useTheme();
  const [selectedTab, setSelectedTab] = useState<Tab>(defaultTab);
  const [fileTree, setFileTree] = useState(defaultFileTree);
  const [createFileOpen, setCreateFileOpen] = useState(false);
  const [deleteFileOpen, setDeleteFileOpen] = useState<string>();
  const [renameFileOpen, setRenameFileOpen] = useState<string>();
  const [dirty, setDirty] = useState(false);
  const [activePath, setActivePath] = useState<string | undefined>(() =>
    findInitialFile(fileTree),
  );

  const triggerPreview = useCallback(() => {
    onPreview(fileTree);
    setSelectedTab("logs");
  }, [fileTree, onPreview]);

  // Stop ctrl+s from saving files and make ctrl+enter trigger a preview.
  useEffect(() => {
    const keyListener = (event: KeyboardEvent) => {
      if (!(navigator.platform.match("Mac") ? event.metaKey : event.ctrlKey)) {
        return;
      }
      switch (event.key) {
        case "s":
          // Prevent opening the save dialog!
          event.preventDefault();
          break;
        case "Enter":
          event.preventDefault();
          triggerPreview();
          break;
      }
    };
    document.addEventListener("keydown", keyListener);
    return () => {
      document.removeEventListener("keydown", keyListener);
    };
  }, [triggerPreview]);

  // Automatically switch to the template preview tab when the build succeeds.
  const previousVersion = useRef<TemplateVersion>();
  useEffect(() => {
    if (!previousVersion.current) {
      previousVersion.current = templateVersion;
      return;
    }

    if (
      ["running", "pending"].includes(previousVersion.current.job.status) &&
      templateVersion.job.status === "succeeded"
    ) {
      setSelectedTab("resources");
      setDirty(false);
    }
    previousVersion.current = templateVersion;
  }, [templateVersion]);

  const hasIcon = template.icon && template.icon !== "";
  const showBuildLogs = Boolean(buildLogs);
  const editorValue = getFileContent(activePath ?? "", fileTree) as string;

  useEffect(() => {
    window.dispatchEvent(new Event("resize"));
  }, [showBuildLogs]);

  return (
    <>
      <DashboardFullPage
        css={{
          background: theme.palette.background.default,
        }}
      >
        <div css={styles.topbar} data-testid="topbar">
          <div css={styles.topbarSides}>
            <Link
              component={RouterLink}
              underline="none"
              to={`/templates/${template.name}`}
            >
              <AvatarData
                title={template.display_name || template.name}
                subtitle={template.description}
                avatar={
                  hasIcon && (
                    <Avatar src={template.icon} variant="square" fitImage />
                  )
                }
              />
            </Link>
          </div>

          {publishedVersion && (
            <Alert
              severity="success"
              dismissible
              actions={
                <Button variant="text" size="small" onClick={onCreateWorkspace}>
                  Create a workspace
                </Button>
              }
            >
              Successfully published {publishedVersion.name}!
            </Alert>
          )}

          <div css={styles.topbarSides}>
            {buildLogs && (
              <TemplateVersionStatusBadge version={templateVersion} />
            )}

            <Button
              title="Build template (Ctrl + Enter)"
              disabled={disablePreview}
              onClick={() => {
                triggerPreview();
              }}
            >
              Build template
            </Button>

            <Button
              variant="contained"
              disabled={dirty || disableUpdate}
              onClick={onPublish}
            >
              Publish version
            </Button>
          </div>
        </div>

        <div css={styles.sidebarAndEditor}>
          <div css={styles.sidebar}>
            <div css={styles.sidebarTitle}>
              Template files
              <div css={styles.sidebarActions}>
                <Tooltip title="Create File" placement="top">
                  <IconButton
                    aria-label="Create File"
                    onClick={(event) => {
                      setCreateFileOpen(true);
                      event.currentTarget.blur();
                    }}
                  >
                    <CreateIcon />
                  </IconButton>
                </Tooltip>
              </div>
              <CreateFileDialog
                fileTree={fileTree}
                open={createFileOpen}
                onClose={() => {
                  setCreateFileOpen(false);
                }}
                checkExists={(path) => existsFile(path, fileTree)}
                onConfirm={(path) => {
                  setFileTree((fileTree) => createFile(path, fileTree, ""));
                  setActivePath(path);
                  setCreateFileOpen(false);
                  setDirty(true);
                }}
              />
              <DeleteFileDialog
                onConfirm={() => {
                  if (!deleteFileOpen) {
                    throw new Error("delete file must be set");
                  }
                  setFileTree((fileTree) =>
                    removeFile(deleteFileOpen, fileTree),
                  );
                  setDeleteFileOpen(undefined);
                  if (activePath === deleteFileOpen) {
                    setActivePath(undefined);
                  }
                  setDirty(true);
                }}
                open={Boolean(deleteFileOpen)}
                onClose={() => setDeleteFileOpen(undefined)}
                filename={deleteFileOpen || ""}
              />
              <RenameFileDialog
                fileTree={fileTree}
                open={Boolean(renameFileOpen)}
                onClose={() => {
                  setRenameFileOpen(undefined);
                }}
                filename={renameFileOpen || ""}
                checkExists={(path) => existsFile(path, fileTree)}
                onConfirm={(newPath) => {
                  if (!renameFileOpen) {
                    return;
                  }
                  setFileTree((fileTree) =>
                    moveFile(renameFileOpen, newPath, fileTree),
                  );
                  setActivePath(newPath);
                  setRenameFileOpen(undefined);
                  setDirty(true);
                }}
              />
            </div>
            <FileTreeView
              fileTree={fileTree}
              onDelete={(file) => setDeleteFileOpen(file)}
              onSelect={(filePath) => {
                if (!isFolder(filePath, fileTree)) {
                  setActivePath(filePath);
                }
              }}
              onRename={(file) => setRenameFileOpen(file)}
              activePath={activePath}
            />
          </div>

          <div
            css={{
              display: "grid",
              width: "100%",
              gridTemplateColumns: showBuildLogs ? "1fr 1fr" : "1fr 0fr",
              minHeight: "100%",
              overflow: "hidden",
            }}
          >
            <div css={styles.editor} data-chromatic="ignore">
              {activePath ? (
                <MonacoEditor
                  value={editorValue}
                  path={activePath}
                  onChange={(value) => {
                    if (!activePath) {
                      return;
                    }
                    setFileTree((fileTree) =>
                      updateFile(activePath, value, fileTree),
                    );
                    setDirty(true);
                  }}
                />
              ) : (
                <div>No file opened</div>
              )}
            </div>

            <div css={styles.panelWrapper}>
              <div css={styles.tabs}>
                <button
                  css={styles.tab}
                  className={selectedTab === "logs" ? "active" : ""}
                  onClick={() => {
                    setSelectedTab("logs");
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
                    css={styles.tab}
                    className={selectedTab === "resources" ? "active" : ""}
                    onClick={() => {
                      setSelectedTab("resources");
                    }}
                  >
                    <PreviewIcon />
                    Workspace Preview
                  </button>
                )}
              </div>

              <div
                css={[
                  styles.panel,
                  {
                    display: selectedTab !== "logs" ? "none" : "flex",
                    flexDirection: "column",
                  },
                ]}
              >
                {templateVersion.job.error && (
                  <div>
                    <Alert
                      severity="error"
                      sx={{
                        borderRadius: 0,
                        border: 0,
                        borderBottom: (theme) =>
                          `1px solid ${theme.palette.divider}`,
                        borderLeft: (theme) =>
                          `2px solid ${theme.palette.error.main}`,
                      }}
                    >
                      <AlertTitle>Error during the build</AlertTitle>
                      <AlertDetail>{templateVersion.job.error}</AlertDetail>
                    </Alert>
                  </div>
                )}

                {buildLogs && buildLogs.length > 0 && (
                  <WorkspaceBuildLogs
                    sx={{ borderRadius: 0, border: 0 }}
                    hideTimestamps
                    logs={buildLogs}
                  />
                )}
              </div>

              <div
                css={[
                  styles.panel,
                  {
                    paddingBottom: theme.spacing(2),
                    display: selectedTab !== "resources" ? "none" : undefined,
                  },
                ]}
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
          </div>
        </div>
      </DashboardFullPage>

      <PublishTemplateVersionDialog
        key={templateVersion.name}
        publishingError={publishingError}
        open={isAskingPublishParameters || isPublishing}
        onClose={onCancelPublish}
        onConfirm={onConfirmPublish}
        isPublishing={isPublishing}
        defaultName={templateVersion.name}
      />

      <MissingTemplateVariablesDialog
        open={isPromptingMissingVariables}
        onClose={onCancelSubmitMissingVariableValues}
        onSubmit={onSubmitMissingVariableValues}
        missingVariables={missingVariables}
      />
    </>
  );
};

const styles = {
  topbar: (theme) => ({
    padding: theme.spacing(2),
    borderBottom: `1px solid ${theme.palette.divider}`,
    display: "flex",
    alignItems: "center",
    justifyContent: "space-between",
    height: topbarHeight,
    background: theme.palette.background.paper,
  }),
  topbarSides: (theme) => ({
    display: "flex",
    alignItems: "center",
    gap: theme.spacing(2),
  }),
  sidebarAndEditor: {
    display: "flex",
    flex: 1,
    flexBasis: 0,
    overflow: "hidden",
  },
  sidebar: (theme) => ({
    minWidth: 256,
    backgroundColor: theme.palette.background.paper,
    borderRight: `1px solid ${theme.palette.divider}`,
  }),
  sidebarTitle: (theme) => ({
    fontSize: 10,
    textTransform: "uppercase",
    padding: theme.spacing(1, 2),
    color: theme.palette.text.primary,
    fontWeight: 500,
    letterSpacing: "0.5px",
    display: "flex",
    alignItems: "center",
  }),
  sidebarActions: (theme) => ({
    marginLeft: "auto",
    "& svg": {
      fill: theme.palette.text.primary,
    },
  }),
  editor: {
    flex: 1,
  },
  panelWrapper: (theme) => ({
    flex: 1,
    borderLeft: `1px solid ${theme.palette.divider}`,
    overflow: "hidden",
    display: "flex",
    flexDirection: "column",
  }),
  panel: {
    overflowY: "auto",
    height: "100%",

    // Hack to access customize resource-card from here
    "& .resource-card": {
      border: 0,
    },
  },
  tabs: (theme) => ({
    borderBottom: `1px solid ${theme.palette.divider}`,
    display: "flex",
    boxShadow: "#000000 0 6px 6px -6px inset",

    "& .MuiTab-root": {
      padding: 0,
      fontSize: 14,
      textTransform: "none",
      letterSpacing: "unset",
    },
  }),
  tab: (theme) => ({
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
  }),
  tabBar: (theme) => ({
    padding: "8px 16px",
    position: "sticky",
    top: 0,
    background: theme.palette.background.default,
    borderBottom: `1px solid ${theme.palette.divider}`,
    color: theme.palette.text.primary,
    textTransform: "uppercase",
    fontSize: 12,

    "&.top": {
      borderTop: `1px solid ${theme.palette.divider}`,
    },
  }),
} satisfies Record<string, Interpolation<Theme>>;
