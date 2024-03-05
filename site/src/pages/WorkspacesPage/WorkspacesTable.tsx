import { useTheme } from "@emotion/react";
import KeyboardArrowRight from "@mui/icons-material/KeyboardArrowRight";
import Star from "@mui/icons-material/Star";
import Checkbox from "@mui/material/Checkbox";
import Skeleton from "@mui/material/Skeleton";
import Table from "@mui/material/Table";
import TableBody from "@mui/material/TableBody";
import TableCell from "@mui/material/TableCell";
import TableContainer from "@mui/material/TableContainer";
import TableHead from "@mui/material/TableHead";
import TableRow from "@mui/material/TableRow";
import type { FC, ReactNode } from "react";
import { useNavigate } from "react-router-dom";
import type { Template, Workspace } from "api/typesGenerated";
import { ExternalAvatar } from "components/Avatar/Avatar";
import { AvatarData } from "components/AvatarData/AvatarData";
import { AvatarDataSkeleton } from "components/AvatarData/AvatarDataSkeleton";
import { InfoTooltip } from "components/InfoTooltip/InfoTooltip";
import { Stack } from "components/Stack/Stack";
import {
  TableLoaderSkeleton,
  TableRowSkeleton,
} from "components/TableLoader/TableLoader";
import { useClickableTableRow } from "hooks/useClickableTableRow";
import { WorkspaceOutdatedTooltip } from "modules/workspaces/WorkspaceOutdatedTooltip/WorkspaceOutdatedTooltip";
import {
  DormantStatusBadge,
  WorkspaceStatusBadge,
} from "modules/workspaces/WorkspaceStatusBadge/WorkspaceStatusBadge";
import { LastUsed } from "pages/WorkspacesPage/LastUsed";
import { getDisplayWorkspaceTemplateName } from "utils/workspace";
import { WorkspacesEmpty } from "./WorkspacesEmpty";

export interface WorkspacesTableProps {
  workspaces?: Workspace[];
  checkedWorkspaces: Workspace[];
  error?: unknown;
  isUsingFilter: boolean;
  onUpdateWorkspace: (workspace: Workspace) => void;
  onCheckChange: (checkedWorkspaces: Workspace[]) => void;
  canCheckWorkspaces: boolean;
  templates?: Template[];
  canCreateTemplate: boolean;
}

export const WorkspacesTable: FC<WorkspacesTableProps> = ({
  workspaces,
  checkedWorkspaces,
  isUsingFilter,
  onUpdateWorkspace,
  onCheckChange,
  canCheckWorkspaces,
  templates,
  canCreateTemplate,
}) => {
  const theme = useTheme();

  return (
    <TableContainer>
      <Table>
        <TableHead>
          <TableRow>
            <TableCell width="40%">
              <div css={{ display: "flex", alignItems: "center", gap: 8 }}>
                {canCheckWorkspaces && (
                  <Checkbox
                    // Remove the extra padding added for the first cell in the
                    // table
                    css={{
                      marginLeft: "-20px",
                      // MUI by default adds 9px padding to enhance the
                      // clickable area. We aim to prevent this from impacting
                      // the layout of surrounding elements.
                      marginTop: -9,
                      marginBottom: -9,
                    }}
                    disabled={!workspaces || workspaces.length === 0}
                    checked={checkedWorkspaces.length === workspaces?.length}
                    size="xsmall"
                    onChange={(_, checked) => {
                      if (!workspaces) {
                        return;
                      }

                      if (!checked) {
                        onCheckChange([]);
                      } else {
                        onCheckChange(workspaces);
                      }
                    }}
                  />
                )}
                Name
              </div>
            </TableCell>
            <TableCell width="25%">Template</TableCell>
            <TableCell width="20%">Last used</TableCell>
            <TableCell width="15%">Status</TableCell>
            <TableCell width="1%" />
          </TableRow>
        </TableHead>
        <TableBody>
          {!workspaces && (
            <TableLoader canCheckWorkspaces={canCheckWorkspaces} />
          )}
          {workspaces && workspaces.length === 0 && (
            <WorkspacesEmpty
              templates={templates}
              isUsingFilter={isUsingFilter}
              canCreateTemplate={canCreateTemplate}
            />
          )}
          {workspaces &&
            workspaces.map((workspace) => {
              const checked = checkedWorkspaces.some(
                (w) => w.id === workspace.id,
              );
              return (
                <WorkspacesRow
                  workspace={workspace}
                  key={workspace.id}
                  checked={checked}
                >
                  <TableCell>
                    <div
                      css={{ display: "flex", alignItems: "center", gap: 8 }}
                    >
                      {canCheckWorkspaces && (
                        <Checkbox
                          // Remove the extra padding added for the first cell in the
                          // table
                          css={{
                            marginLeft: "-20px",
                          }}
                          data-testid={`checkbox-${workspace.id}`}
                          size="xsmall"
                          disabled={cantBeChecked(workspace)}
                          checked={checked}
                          onClick={(e) => {
                            e.stopPropagation();
                          }}
                          onChange={(e) => {
                            if (e.currentTarget.checked) {
                              onCheckChange([...checkedWorkspaces, workspace]);
                            } else {
                              onCheckChange(
                                checkedWorkspaces.filter(
                                  (w) => w.id !== workspace.id,
                                ),
                              );
                            }
                          }}
                        />
                      )}
                      <AvatarData
                        title={
                          <Stack
                            direction="row"
                            spacing={0.5}
                            alignItems="center"
                          >
                            {workspace.name}
                            {workspace.favorite && (
                              <Star css={{ width: 16, height: 16 }} />
                            )}
                            {workspace.outdated && (
                              <WorkspaceOutdatedTooltip
                                templateName={workspace.template_name}
                                latestVersionId={
                                  workspace.template_active_version_id
                                }
                                onUpdateVersion={() => {
                                  onUpdateWorkspace(workspace);
                                }}
                              />
                            )}
                          </Stack>
                        }
                        subtitle={workspace.owner_name}
                        avatar={
                          <ExternalAvatar
                            src={workspace.template_icon}
                            variant={
                              workspace.template_icon ? "square" : undefined
                            }
                            fitImage={Boolean(workspace.template_icon)}
                          >
                            {workspace.name}
                          </ExternalAvatar>
                        }
                      />
                    </div>
                  </TableCell>

                  <TableCell>
                    {getDisplayWorkspaceTemplateName(workspace)}
                  </TableCell>

                  <TableCell>
                    <LastUsed lastUsedAt={workspace.last_used_at} />
                  </TableCell>

                  <TableCell>
                    <div
                      css={{ display: "flex", alignItems: "center", gap: 8 }}
                    >
                      <WorkspaceStatusBadge workspace={workspace} />
                      {workspace.latest_build.status === "running" &&
                        !workspace.health.healthy && (
                          <InfoTooltip
                            type="notice"
                            title="Workspace is unhealthy"
                            message="Your workspace is running but some agents are unhealthy."
                          />
                        )}
                      {workspace.dormant_at && (
                        <DormantStatusBadge workspace={workspace} />
                      )}
                    </div>
                  </TableCell>

                  <TableCell>
                    <div css={{ display: "flex", paddingLeft: 16 }}>
                      <KeyboardArrowRight
                        css={{
                          color: theme.palette.text.secondary,
                          width: 20,
                          height: 20,
                        }}
                      />
                    </div>
                  </TableCell>
                </WorkspacesRow>
              );
            })}
        </TableBody>
      </Table>
    </TableContainer>
  );
};

interface WorkspacesRowProps {
  workspace: Workspace;
  children?: ReactNode;
  checked: boolean;
}

const WorkspacesRow: FC<WorkspacesRowProps> = ({
  workspace,
  children,
  checked,
}) => {
  const navigate = useNavigate();
  const theme = useTheme();

  const workspacePageLink = `/@${workspace.owner_name}/${workspace.name}`;
  const openLinkInNewTab = () => window.open(workspacePageLink, "_blank");
  const clickableProps = useClickableTableRow({
    onMiddleClick: openLinkInNewTab,
    onClick: (event) => {
      // Order of booleans actually matters here for Windows-Mac compatibility;
      // meta key is Cmd on Macs, but on Windows, it's either the Windows key,
      // or the key does nothing at all (depends on the browser)
      const shouldOpenInNewTab =
        event.shiftKey || event.metaKey || event.ctrlKey;

      if (shouldOpenInNewTab) {
        openLinkInNewTab();
      } else {
        navigate(workspacePageLink);
      }
    },
  });

  const bgColor = checked ? theme.palette.action.hover : undefined;

  return (
    <TableRow
      {...clickableProps}
      data-testid={`workspace-${workspace.id}`}
      css={{
        ...clickableProps.css,
        backgroundColor: bgColor,

        "&:hover": {
          backgroundColor: `${bgColor} !important`,
        },
      }}
    >
      {children}
    </TableRow>
  );
};

interface TableLoaderProps {
  canCheckWorkspaces?: boolean;
}

const TableLoader: FC<TableLoaderProps> = ({ canCheckWorkspaces }) => {
  return (
    <TableLoaderSkeleton>
      <TableRowSkeleton>
        <TableCell width="40%">
          <div css={{ display: "flex", alignItems: "center", gap: 8 }}>
            {canCheckWorkspaces && (
              <Checkbox size="small" disabled css={{ marginLeft: "-20px" }} />
            )}
            <AvatarDataSkeleton />
          </div>
        </TableCell>
        <TableCell>
          <Skeleton variant="text" width="25%" />
        </TableCell>
        <TableCell>
          <Skeleton variant="text" width="25%" />
        </TableCell>
        <TableCell>
          <Skeleton variant="text" width="25%" />
        </TableCell>
        <TableCell>
          <Skeleton variant="text" width="25%" />
        </TableCell>
      </TableRowSkeleton>
    </TableLoaderSkeleton>
  );
};

const cantBeChecked = (workspace: Workspace) => {
  return ["deleting", "pending"].includes(workspace.latest_build.status);
};
