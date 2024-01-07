import Button from "@mui/material/Button";
import IconButton from "@mui/material/IconButton";
import Tooltip from "@mui/material/Tooltip";
import CreateIcon from "@mui/icons-material/AddOutlined";
import type {
  ProvisionerJobLog,
  Template,
  TemplateVersion,
  TemplateVersionVariable,
  VariableValue,
  WorkspaceResource,
} from "api/typesGenerated";
import { Link as RouterLink } from "react-router-dom";
import { Alert, AlertDetail } from "components/Alert/Alert";
import { TemplateResourcesTable } from "components/TemplateResourcesTable/TemplateResourcesTable";
import { WorkspaceBuildLogs } from "components/WorkspaceBuildLogs/WorkspaceBuildLogs";
import { PublishVersionData } from "pages/TemplateVersionEditorPage/types";
import { type FC, useCallback, useEffect, useRef, useState } from "react";
import PlayArrowOutlined from "@mui/icons-material/PlayArrowOutlined";
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
import { TemplateVersionStatusBadge } from "./TemplateVersionStatusBadge";
import AlertTitle from "@mui/material/AlertTitle";
import { type Interpolation, type Theme, useTheme } from "@emotion/react";
import ArrowBackOutlined from "@mui/icons-material/ArrowBackOutlined";
import CloseOutlined from "@mui/icons-material/CloseOutlined";
import { MONOSPACE_FONT_FAMILY } from "theme/constants";
import { Loader } from "components/Loader/Loader";
import {
  Topbar,
  TopbarAvatar,
  TopbarButton,
  TopbarData,
  TopbarDivider,
  TopbarIconButton,
} from "components/FullPageLayout/Topbar";
import { Sidebar } from "components/FullPageLayout/Sidebar";

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
      setDirty(false);
    }
    previousVersion.current = templateVersion;
  }, [templateVersion]);

  const editorValue = getFileContent(activePath ?? "", fileTree) as string;

  // Auto scroll
  const buildLogsRef = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (buildLogsRef.current) {
      buildLogsRef.current.scrollTop = buildLogsRef.current.scrollHeight;
    }
  }, [buildLogs]);

  return (
    <>
      <div css={{ height: "100%", display: "flex", flexDirection: "column" }}>
        <Topbar
          css={{
            display: "grid",
            gridTemplateColumns: "1fr 2fr 1fr",
          }}
          data-testid="topbar"
        >
          <div>
            <Tooltip title="Back to the template">
              <TopbarIconButton
                component={RouterLink}
                to={`/templates/${template.name}`}
              >
                <ArrowBackOutlined />
              </TopbarIconButton>
            </Tooltip>
          </div>

          <TopbarData>
            <TopbarAvatar src={template.icon} />
            <RouterLink
              to={`/templates/${template.name}`}
              css={{
                color: theme.palette.text.primary,
                textDecoration: "none",

                "&:hover": {
                  textDecoration: "underline",
                },
              }}
            >
              {template.display_name || template.name}
            </RouterLink>
            <TopbarDivider />
            <span css={{ color: theme.palette.text.secondary }}>
              {templateVersion.name}
            </span>
          </TopbarData>

          <div
            css={{
              display: "flex",
              alignItems: "center",
              justifyContent: "flex-end",
              gap: 8,
              paddingRight: 16,
            }}
          >
            {buildLogs && (
              <TemplateVersionStatusBadge version={templateVersion} />
            )}

            <TopbarButton
              startIcon={
                <PlayArrowOutlined
                  css={{ color: theme.palette.success.light }}
                />
              }
              title="Build template (Ctrl + Enter)"
              disabled={disablePreview}
              onClick={() => {
                triggerPreview();
              }}
            >
              Build
            </TopbarButton>

            <TopbarButton
              variant="contained"
              disabled={dirty || disableUpdate}
              onClick={onPublish}
            >
              Publish
            </TopbarButton>
          </div>
        </Topbar>

        <div
          css={{
            display: "flex",
            flex: 1,
            flexBasis: 0,
            overflow: "hidden",
            position: "relative",
          }}
        >
          {publishedVersion && (
            <div
              // We need this to reset the dismissable state of the component
              // when the published version changes
              key={publishedVersion.id}
              css={{
                position: "absolute",
                width: "100%",
                display: "flex",
                justifyContent: "center",
                padding: 12,
                zIndex: 10,
              }}
            >
              <Alert
                severity="success"
                dismissible
                actions={
                  <Button
                    variant="text"
                    size="small"
                    onClick={onCreateWorkspace}
                  >
                    Create a workspace
                  </Button>
                }
              >
                Successfully published {publishedVersion.name}!
              </Alert>
            </div>
          )}

          <Sidebar>
            <div
              css={{
                height: 42,
                padding: "0 8px 0 16px",
                display: "flex",
                alignItems: "center",
              }}
            >
              <span
                css={{
                  color: theme.palette.text.primary,
                  fontSize: 13,
                }}
              >
                Files
              </span>

              <div
                css={{
                  marginLeft: "auto",
                  "& svg": {
                    fill: theme.palette.text.primary,
                  },
                }}
              >
                <Tooltip title="Create File" placement="top">
                  <IconButton
                    aria-label="Create File"
                    onClick={(event) => {
                      setCreateFileOpen(true);
                      event.currentTarget.blur();
                    }}
                  >
                    <CreateIcon css={{ width: 16, height: 16 }} />
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
          </Sidebar>

          <div
            css={{
              display: "flex",
              flexDirection: "column",
              width: "100%",
              minHeight: "100%",
              overflow: "hidden",
            }}
          >
            <div css={{ flex: 1, overflowY: "auto" }} data-chromatic="ignore">
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

            <div
              css={{
                borderTop: `1px solid ${theme.palette.divider}`,
                overflow: "hidden",
                display: "flex",
                flexDirection: "column",
              }}
            >
              <div
                css={{
                  display: "flex",
                  alignItems: "center",
                  borderBottom: selectedTab
                    ? `1px solid ${theme.palette.divider}`
                    : 0,
                }}
              >
                <div
                  css={{
                    display: "flex",

                    "& .MuiTab-root": {
                      padding: 0,
                      fontSize: 14,
                      textTransform: "none",
                      letterSpacing: "unset",
                    },
                  }}
                >
                  <button
                    disabled={!buildLogs}
                    css={styles.tab}
                    className={selectedTab === "logs" ? "active" : ""}
                    onClick={() => {
                      setSelectedTab("logs");
                    }}
                  >
                    Output
                  </button>

                  <button
                    disabled={disableUpdate}
                    css={styles.tab}
                    className={selectedTab === "resources" ? "active" : ""}
                    onClick={() => {
                      setSelectedTab("resources");
                    }}
                  >
                    Resources
                  </button>
                </div>

                {selectedTab && (
                  <IconButton
                    onClick={() => {
                      setSelectedTab(undefined);
                    }}
                    css={{
                      marginLeft: "auto",
                      width: 36,
                      height: 36,
                      borderRadius: 0,
                    }}
                  >
                    <CloseOutlined css={{ width: 16, height: 16 }} />
                  </IconButton>
                )}
              </div>

              <div
                ref={buildLogsRef}
                css={{
                  display: selectedTab !== "logs" ? "none" : "flex",
                  flexDirection: "column",
                  overflowY: "auto",
                  height: selectedTab ? 280 : 0,
                }}
              >
                {templateVersion.job.error && (
                  <div>
                    <Alert
                      severity="error"
                      css={{
                        borderRadius: 0,
                        border: 0,
                        borderBottom: `1px solid ${theme.palette.divider}`,
                        borderLeft: `2px solid ${theme.palette.error.main}`,
                      }}
                    >
                      <AlertTitle>Error during the build</AlertTitle>
                      <AlertDetail>{templateVersion.job.error}</AlertDetail>
                    </Alert>
                  </div>
                )}

                {buildLogs && buildLogs.length === 0 && (
                  <Loader css={{ height: "100%" }} />
                )}

                {buildLogs && buildLogs.length > 0 && (
                  <WorkspaceBuildLogs
                    css={{
                      borderRadius: 0,
                      border: 0,

                      // Hack to update logs header and lines
                      "& .logs-header": {
                        border: 0,
                        padding: "0 16px",
                        fontFamily: MONOSPACE_FONT_FAMILY,

                        "&:first-child": {
                          paddingTop: 16,
                        },

                        "&:last-child": {
                          paddingBottom: 16,
                        },
                      },

                      "& .logs-line": {
                        paddingLeft: 16,
                      },

                      "& .logs-container": {
                        border: "0 !important",
                      },
                    }}
                    hideTimestamps
                    logs={buildLogs}
                  />
                )}
              </div>

              <div
                css={{
                  display: selectedTab !== "resources" ? "none" : undefined,
                  overflowY: "auto",
                  height: selectedTab ? 280 : 0,

                  // Hack to access customize resource-card from here
                  "& .resource-card": {
                    borderLeft: 0,
                    borderRight: 0,

                    "&:first-child": {
                      borderTop: 0,
                    },

                    "&:last-child": {
                      borderBottom: 0,
                    },
                  },
                }}
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
      </div>

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
  tab: (theme) => ({
    "&:not(:disabled)": {
      cursor: "pointer",
    },
    padding: 12,
    fontSize: 10,
    textTransform: "uppercase",
    letterSpacing: "0.5px",
    fontWeight: 500,
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
        backgroundColor: theme.palette.primary.main,
        bottom: -1,
        position: "absolute",
      },
    },

    "&:not(:disabled):hover": {
      color: theme.palette.text.primary,
    },

    "&:disabled": {
      color: theme.palette.text.disabled,
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
