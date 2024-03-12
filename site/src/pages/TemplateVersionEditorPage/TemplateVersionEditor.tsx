import { type Interpolation, type Theme, useTheme } from "@emotion/react";
import CreateIcon from "@mui/icons-material/AddOutlined";
import ArrowBackOutlined from "@mui/icons-material/ArrowBackOutlined";
import CloseOutlined from "@mui/icons-material/CloseOutlined";
import PlayArrowOutlined from "@mui/icons-material/PlayArrowOutlined";
import WarningOutlined from "@mui/icons-material/WarningOutlined";
import AlertTitle from "@mui/material/AlertTitle";
import Button from "@mui/material/Button";
import ButtonGroup from "@mui/material/ButtonGroup";
import IconButton from "@mui/material/IconButton";
import Tooltip from "@mui/material/Tooltip";
import { type FC, useCallback, useEffect, useRef, useState } from "react";
import {
  Link as RouterLink,
  unstable_usePrompt as usePrompt,
} from "react-router-dom";
import type {
  ProvisionerJobLog,
  Template,
  TemplateVersion,
  TemplateVersionVariable,
  VariableValue,
  WorkspaceResource,
} from "api/typesGenerated";
import { Alert, AlertDetail } from "components/Alert/Alert";
import { Sidebar } from "components/FullPageLayout/Sidebar";
import {
  Topbar,
  TopbarAvatar,
  TopbarButton,
  TopbarData,
  TopbarDivider,
  TopbarIconButton,
} from "components/FullPageLayout/Topbar";
import { Loader } from "components/Loader/Loader";
import { isBinaryData } from "modules/templates/TemplateFiles/isBinaryData";
import { TemplateFileTree } from "modules/templates/TemplateFiles/TemplateFileTree";
import { TemplateResourcesTable } from "modules/templates/TemplateResourcesTable/TemplateResourcesTable";
import { WorkspaceBuildLogs } from "modules/workspaces/WorkspaceBuildLogs/WorkspaceBuildLogs";
import type { PublishVersionData } from "pages/TemplateVersionEditorPage/types";
import { MONOSPACE_FONT_FAMILY } from "theme/constants";
import {
  createFile,
  existsFile,
  type FileTree,
  getFileText,
  isFolder,
  moveFile,
  removeFile,
  updateFile,
} from "utils/filetree";
import {
  CreateFileDialog,
  DeleteFileDialog,
  RenameFileDialog,
} from "./FileDialog";
import { MissingTemplateVariablesDialog } from "./MissingTemplateVariablesDialog";
import { MonacoEditor } from "./MonacoEditor";
import { ProvisionerTagsPopover } from "./ProvisionerTagsPopover";
import { PublishTemplateVersionDialog } from "./PublishTemplateVersionDialog";
import { TemplateVersionStatusBadge } from "./TemplateVersionStatusBadge";

type Tab = "logs" | "resources" | undefined; // Undefined is to hide the tab

export interface TemplateVersionEditorProps {
  template: Template;
  templateVersion: TemplateVersion;
  defaultFileTree: FileTree;
  buildLogs?: ProvisionerJobLog[];
  resources?: WorkspaceResource[];
  isBuilding: boolean;
  canPublish: boolean;
  onPreview: (files: FileTree) => Promise<void>;
  onPublish: () => void;
  onConfirmPublish: (data: PublishVersionData) => void;
  onCancelPublish: () => void;
  publishingError?: unknown;
  publishedVersion?: TemplateVersion;
  onCreateWorkspace: () => void;
  isAskingPublishParameters: boolean;
  isPromptingMissingVariables: boolean;
  isPublishing: boolean;
  missingVariables?: TemplateVersionVariable[];
  onSubmitMissingVariableValues: (values: VariableValue[]) => void;
  onCancelSubmitMissingVariableValues: () => void;
  defaultTab?: Tab;
  provisionerTags: Record<string, string>;
  onUpdateProvisionerTags: (tags: Record<string, string>) => void;
  activePath: string | undefined;
  onActivePathChange: (path: string | undefined) => void;
}

export const TemplateVersionEditor: FC<TemplateVersionEditorProps> = ({
  isBuilding,
  canPublish,
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
  provisionerTags,
  onUpdateProvisionerTags,
  activePath,
  onActivePathChange,
}) => {
  const theme = useTheme();
  const [selectedTab, setSelectedTab] = useState<Tab>(defaultTab);
  const [fileTree, setFileTree] = useState(defaultFileTree);
  const [createFileOpen, setCreateFileOpen] = useState(false);
  const [deleteFileOpen, setDeleteFileOpen] = useState<string>();
  const [renameFileOpen, setRenameFileOpen] = useState<string>();
  const [dirty, setDirty] = useState(false);

  const triggerPreview = useCallback(async () => {
    await onPreview(fileTree);
    setSelectedTab("logs");
  }, [fileTree, onPreview]);

  // Stop ctrl+s from saving files and make ctrl+enter trigger a preview.
  useEffect(() => {
    const keyListener = async (event: KeyboardEvent) => {
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
          await triggerPreview();
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

  const editorValue = activePath ? getFileText(activePath, fileTree) : "";
  const isEditorValueBinary =
    typeof editorValue === "string" ? isBinaryData(editorValue) : false;

  // Auto scroll
  const buildLogsRef = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (buildLogsRef.current) {
      buildLogsRef.current.scrollTop = buildLogsRef.current.scrollHeight;
    }
  }, [buildLogs]);

  useLeaveSiteWarning(canPublish);

  const canBuild = !isBuilding;

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

            <ButtonGroup
              variant="outlined"
              css={{
                // Workaround to make the border transitions smoothly on button groups
                "& > button:hover + button": {
                  borderLeft: "1px solid #FFF",
                },
              }}
              disabled={!canBuild}
            >
              <TopbarButton
                startIcon={
                  <PlayArrowOutlined
                    css={{ color: theme.palette.success.light }}
                  />
                }
                title="Build template (Ctrl + Enter)"
                disabled={!canBuild}
                onClick={async () => {
                  await triggerPreview();
                }}
              >
                Build
              </TopbarButton>
              <ProvisionerTagsPopover
                tags={provisionerTags}
                onSubmit={({ key, value }) => {
                  onUpdateProvisionerTags({
                    ...provisionerTags,
                    [key]: value,
                  });
                }}
                onDelete={(key) => {
                  const newTags = { ...provisionerTags };
                  delete newTags[key];
                  onUpdateProvisionerTags(newTags);
                }}
              />
            </ButtonGroup>

            <TopbarButton
              variant="contained"
              disabled={dirty || !canPublish}
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
                  onActivePathChange(path);
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
                    onActivePathChange(undefined);
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
                  onActivePathChange(newPath);
                  setRenameFileOpen(undefined);
                  setDirty(true);
                }}
              />
            </div>
            <TemplateFileTree
              fileTree={fileTree}
              onDelete={(file) => setDeleteFileOpen(file)}
              onSelect={(filePath) => {
                if (!isFolder(filePath, fileTree)) {
                  onActivePathChange(filePath);
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
                isEditorValueBinary ? (
                  <div
                    role="alert"
                    css={{
                      width: "100%",
                      height: "100%",
                      display: "flex",
                      alignItems: "center",
                      justifyContent: "center",
                      padding: 40,
                    }}
                  >
                    <div
                      css={{
                        display: "flex",
                        flexDirection: "column",
                        alignItems: "center",
                        maxWidth: 420,
                        textAlign: "center",
                      }}
                    >
                      <WarningOutlined
                        css={{
                          fontSize: 48,
                          color: theme.roles.warning.fill.outline,
                        }}
                      />
                      <p
                        css={{
                          margin: 0,
                          padding: 0,
                          marginTop: 24,
                        }}
                      >
                        The file is not displayed in the text editor because it
                        is either binary or uses an unsupported text encoding.
                      </p>
                    </div>
                  </div>
                ) : (
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
                )
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
                    disabled={!canPublish}
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
                  height: selectedTab ? 280 : 0,
                  flexDirection: "column",
                  overflowY: "auto",
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
                    css={styles.buildLogs}
                    hideTimestamps
                    logs={buildLogs}
                  />
                )}
              </div>

              <div
                css={[
                  {
                    display: selectedTab !== "resources" ? "none" : undefined,
                    height: selectedTab ? 280 : 0,
                  },
                  styles.resources,
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

const useLeaveSiteWarning = (enabled: boolean) => {
  const MESSAGE =
    "You have unpublished changes. Are you sure you want to leave?";

  // This works for regular browser actions like close tab and back button
  useEffect(() => {
    const onBeforeUnload = (e: BeforeUnloadEvent) => {
      if (enabled) {
        e.preventDefault();
        return MESSAGE;
      }
    };

    window.addEventListener("beforeunload", onBeforeUnload);

    return () => {
      window.removeEventListener("beforeunload", onBeforeUnload);
    };
  }, [enabled]);

  // This is used for react router navigation that is not triggered by the
  // browser
  usePrompt({
    message: MESSAGE,
    when: ({ nextLocation }) => {
      // We need to check the path because we change the URL when new template
      // version is created during builds
      return enabled && !nextLocation.pathname.endsWith("/edit");
    },
  });
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

  buildLogs: {
    borderRadius: 0,
    border: 0,

    // Hack to update logs header and lines
    "& .logs-header": {
      border: 0,
      padding: "8px 16px",
      fontFamily: MONOSPACE_FONT_FAMILY,

      "&:first-of-type": {
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
  },

  resources: {
    overflowY: "auto",

    // Hack to access customize resource-card from here
    "& .resource-card": {
      borderLeft: 0,
      borderRight: 0,

      "&:first-of-type": {
        borderTop: 0,
      },

      "&:last-child": {
        borderBottom: 0,
      },
    },
  },
} satisfies Record<string, Interpolation<Theme>>;
