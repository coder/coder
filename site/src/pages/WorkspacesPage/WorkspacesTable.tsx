import Table from "@mui/material/Table";
import TableBody from "@mui/material/TableBody";
import TableCell from "@mui/material/TableCell";
import TableContainer from "@mui/material/TableContainer";
import TableHead from "@mui/material/TableHead";
import TableRow from "@mui/material/TableRow";
import { Template, Workspace } from "api/typesGenerated";
import { FC, ReactNode } from "react";
import {
  TableLoaderSkeleton,
  TableRowSkeleton,
} from "components/TableLoader/TableLoader";
import { useNavigate } from "react-router-dom";
import { useClickableTableRow } from "hooks/useClickableTableRow";
import KeyboardArrowRight from "@mui/icons-material/KeyboardArrowRight";
import Box from "@mui/material/Box";
import { AvatarData } from "components/AvatarData/AvatarData";
import { Avatar } from "components/Avatar/Avatar";
import { Stack } from "components/Stack/Stack";
import { LastUsed } from "pages/WorkspacesPage/LastUsed";
import { WorkspaceOutdatedTooltip } from "components/WorkspaceOutdatedTooltip/WorkspaceOutdatedTooltip";
import { WorkspaceStatusBadge } from "components/WorkspaceStatusBadge/WorkspaceStatusBadge";
import { getDisplayWorkspaceTemplateName } from "utils/workspace";
import Checkbox from "@mui/material/Checkbox";
import { AvatarDataSkeleton } from "components/AvatarData/AvatarDataSkeleton";
import Skeleton from "@mui/material/Skeleton";
import { InfoTooltip } from "components/InfoTooltip/InfoTooltip";
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
  return (
    <TableContainer>
      <Table>
        <TableHead>
          <TableRow>
            <TableCell width="40%">
              <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
                {canCheckWorkspaces && (
                  <Checkbox
                    // Remove the extra padding added for the first cell in the
                    // table
                    sx={{ marginLeft: "-20px" }}
                    disabled={!workspaces || workspaces.length === 0}
                    checked={checkedWorkspaces.length === workspaces?.length}
                    size="small"
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
              </Box>
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
                    <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
                      {canCheckWorkspaces && (
                        <Checkbox
                          // Remove the extra padding added for the first cell in the
                          // table
                          sx={{ marginLeft: "-20px" }}
                          data-testid={`checkbox-${workspace.id}`}
                          size="small"
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
                            spacing={0}
                            alignItems="center"
                          >
                            {workspace.name}
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
                          <Avatar
                            src={workspace.template_icon}
                            variant={
                              workspace.template_icon ? "square" : undefined
                            }
                            fitImage={Boolean(workspace.template_icon)}
                          >
                            {workspace.name}
                          </Avatar>
                        }
                      />
                    </Box>
                  </TableCell>

                  <TableCell>
                    {getDisplayWorkspaceTemplateName(workspace)}
                  </TableCell>

                  <TableCell>
                    <LastUsed lastUsedAt={workspace.last_used_at} />
                  </TableCell>

                  <TableCell>
                    <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
                      <WorkspaceStatusBadge workspace={workspace} />
                      {workspace.latest_build.status === "running" &&
                        !workspace.health.healthy && (
                          <InfoTooltip
                            type="warning"
                            title="Workspace is unhealthy"
                            message="Your workspace is running but some agents are unhealthy."
                          />
                        )}
                    </Box>
                  </TableCell>

                  <TableCell>
                    <Box
                      sx={{
                        display: "flex",
                        paddingLeft: (theme) => theme.spacing(2),
                      }}
                    >
                      <KeyboardArrowRight
                        sx={{
                          color: (theme) => theme.palette.text.secondary,
                          width: 20,
                          height: 20,
                        }}
                      />
                    </Box>
                  </TableCell>
                </WorkspacesRow>
              );
            })}
        </TableBody>
      </Table>
    </TableContainer>
  );
};

const WorkspacesRow: FC<{
  workspace: Workspace;
  children: ReactNode;
  checked: boolean;
}> = ({ workspace, children, checked }) => {
  const navigate = useNavigate();

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

  return (
    <TableRow
      {...clickableProps}
      data-testid={`workspace-${workspace.id}`}
      sx={{
        backgroundColor: (theme) =>
          checked ? theme.palette.action.hover : undefined,
      }}
    >
      {children}
    </TableRow>
  );
};

const TableLoader = ({
  canCheckWorkspaces,
}: {
  canCheckWorkspaces: boolean;
}) => {
  return (
    <TableLoaderSkeleton>
      <TableRowSkeleton>
        <TableCell width="40%">
          <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
            {canCheckWorkspaces && (
              <Checkbox size="small" disabled sx={{ marginLeft: "-20px" }} />
            )}
            <AvatarDataSkeleton />
          </Box>
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
