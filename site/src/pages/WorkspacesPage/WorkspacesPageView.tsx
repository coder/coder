import { Template, Workspace } from "api/typesGenerated";
import { PaginationWidgetBase } from "components/PaginationWidget/PaginationWidgetBase";
import { ComponentProps } from "react";
import { Margins } from "components/Margins/Margins";
import { PageHeader, PageHeaderTitle } from "components/PageHeader/PageHeader";
import { Stack } from "components/Stack/Stack";
import { WorkspaceHelpTooltip } from "./WorkspaceHelpTooltip";
import { WorkspacesTable } from "pages/WorkspacesPage/WorkspacesTable";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { WorkspacesFilter } from "./filter/filter";
import { hasError, isApiValidationError } from "api/errors";
import { TableToolbar } from "components/TableToolbar/TableToolbar";
import DeleteOutlined from "@mui/icons-material/DeleteOutlined";
import { WorkspacesButton } from "./WorkspacesButton";
import { UseQueryResult } from "react-query";
import StopOutlined from "@mui/icons-material/StopOutlined";
import PlayArrowOutlined from "@mui/icons-material/PlayArrowOutlined";
import CloudQueue from "@mui/icons-material/CloudQueue";
import {
  MoreMenu,
  MoreMenuContent,
  MoreMenuItem,
  MoreMenuTrigger,
} from "components/MoreMenu/MoreMenu";
import KeyboardArrowDownOutlined from "@mui/icons-material/KeyboardArrowDownOutlined";
import Divider from "@mui/material/Divider";
import LoadingButton from "@mui/lab/LoadingButton";
import { PaginationHeader } from "components/PaginationWidget/PaginationHeader";
import { mustUpdateWorkspace } from "utils/workspace";

export const Language = {
  pageTitle: "Workspaces",
  yourWorkspacesButton: "Your workspaces",
  allWorkspacesButton: "All workspaces",
  runningWorkspacesButton: "Running workspaces",
  createWorkspace: <>Create Workspace&hellip;</>,
  seeAllTemplates: "See all templates",
  template: "Template",
};

type TemplateQuery = UseQueryResult<Template[]>;

export interface WorkspacesPageViewProps {
  error: unknown;
  workspaces?: Workspace[];
  checkedWorkspaces: Workspace[];
  count?: number;
  filterProps: ComponentProps<typeof WorkspacesFilter>;
  page: number;
  limit: number;
  onPageChange: (page: number) => void;
  onUpdateWorkspace: (workspace: Workspace) => void;
  onCheckChange: (checkedWorkspaces: Workspace[]) => void;
  isRunningBatchAction: boolean;
  onDeleteAll: () => void;
  onUpdateAll: () => void;
  onStartAll: () => void;
  onStopAll: () => void;
  canCheckWorkspaces: boolean;
  templatesFetchStatus: TemplateQuery["status"];
  templates: TemplateQuery["data"];
  canCreateTemplate: boolean;
  canChangeVersions: boolean;
}

export const WorkspacesPageView = ({
  workspaces,
  error,
  limit,
  count,
  filterProps,
  onPageChange,
  onUpdateWorkspace,
  page,
  checkedWorkspaces,
  onCheckChange,
  onDeleteAll,
  onUpdateAll,
  onStopAll,
  onStartAll,
  isRunningBatchAction,
  canCheckWorkspaces,
  templates,
  templatesFetchStatus,
  canCreateTemplate,
  canChangeVersions,
}: WorkspacesPageViewProps) => {
  return (
    <Margins>
      <PageHeader
        actions={
          <WorkspacesButton
            templates={templates}
            templatesFetchStatus={templatesFetchStatus}
          >
            {Language.createWorkspace}
          </WorkspacesButton>
        }
      >
        <PageHeaderTitle>
          <Stack direction="row" spacing={1} alignItems="center">
            <span>{Language.pageTitle}</span>
            <WorkspaceHelpTooltip />
          </Stack>
        </PageHeaderTitle>
      </PageHeader>

      <Stack>
        {hasError(error) && !isApiValidationError(error) && (
          <ErrorAlert error={error} />
        )}
        <WorkspacesFilter error={error} {...filterProps} />
      </Stack>

      <TableToolbar>
        {checkedWorkspaces.length > 0 ? (
          <>
            <div>
              Selected <strong>{checkedWorkspaces.length}</strong> of{" "}
              <strong>{workspaces?.length}</strong>{" "}
              {workspaces?.length === 1 ? "workspace" : "workspaces"}
            </div>

            <MoreMenu>
              <MoreMenuTrigger>
                <LoadingButton
                  loading={isRunningBatchAction}
                  loadingPosition="end"
                  variant="text"
                  size="small"
                  css={{ borderRadius: 9999, marginLeft: "auto" }}
                  endIcon={<KeyboardArrowDownOutlined />}
                >
                  Actions
                </LoadingButton>
              </MoreMenuTrigger>
              <MoreMenuContent>
                <MoreMenuItem
                  onClick={onStartAll}
                  disabled={
                    !checkedWorkspaces?.every(
                      (w) =>
                        w.latest_build.status === "stopped" &&
                        !mustUpdateWorkspace(w, canChangeVersions),
                    )
                  }
                >
                  <PlayArrowOutlined /> Start
                </MoreMenuItem>
                <MoreMenuItem
                  onClick={onStopAll}
                  disabled={
                    !checkedWorkspaces?.every(
                      (w) => w.latest_build.status === "running",
                    )
                  }
                >
                  <StopOutlined /> Stop
                </MoreMenuItem>
                <Divider />
                <MoreMenuItem onClick={onUpdateAll}>
                  <CloudQueue /> Update&hellip;
                </MoreMenuItem>
                <MoreMenuItem danger onClick={onDeleteAll}>
                  <DeleteOutlined /> Delete&hellip;
                </MoreMenuItem>
              </MoreMenuContent>
            </MoreMenu>
          </>
        ) : (
          <PaginationHeader
            paginationUnitLabel="workspaces"
            limit={limit}
            totalRecords={count}
            currentOffsetStart={(page - 1) * limit + 1}
            css={{ paddingBottom: "0" }}
          />
        )}
      </TableToolbar>

      <WorkspacesTable
        canCreateTemplate={canCreateTemplate}
        workspaces={workspaces}
        isUsingFilter={filterProps.filter.used}
        onUpdateWorkspace={onUpdateWorkspace}
        checkedWorkspaces={checkedWorkspaces}
        onCheckChange={onCheckChange}
        canCheckWorkspaces={canCheckWorkspaces}
        templates={templates}
      />

      {count !== undefined && (
        // Temporary styling stopgap before component is migrated to using
        // PaginationContainer (which renders PaginationWidgetBase using CSS
        // flexbox gaps)
        <div css={{ paddingTop: "16px" }}>
          <PaginationWidgetBase
            totalRecords={count}
            pageSize={limit}
            onPageChange={onPageChange}
            currentPage={page}
          />
        </div>
      )}
    </Margins>
  );
};
