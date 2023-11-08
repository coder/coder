import { usePagination } from "hooks/usePagination";
import { Workspace } from "api/typesGenerated";
import {
  useDashboard,
  useIsWorkspaceActionsEnabled,
} from "components/Dashboard/DashboardProvider";
import { type FC, useEffect, useState } from "react";
import { Helmet } from "react-helmet-async";
import { pageTitle } from "utils/page";
import { useWorkspacesData, useWorkspaceUpdate } from "./data";
import { WorkspacesPageView } from "./WorkspacesPageView";
import { useOrganizationId, usePermissions } from "hooks";
import { useTemplateFilterMenu, useStatusFilterMenu } from "./filter/menus";
import { useSearchParams } from "react-router-dom";
import { useFilter } from "components/Filter/filter";
import { useUserFilterMenu } from "components/Filter/UserFilter";
import { getWorkspaces } from "api/api";
import { useEffectEvent } from "hooks/hookPolyfills";
import { useQuery } from "react-query";
import { templates } from "api/queries/templates";
import { BatchDeleteConfirmation, useBatchActions } from "./BatchActions";

function useSafeSearchParams() {
  // Have to wrap setSearchParams because React Router doesn't make sure that
  // the function's memory reference stays stable on each render, even though
  // its logic never changes, and even though it has function update support
  const [searchParams, setSearchParams] = useSearchParams();
  const stableSetSearchParams = useEffectEvent(setSearchParams);

  // Need this to be a tuple type, but can't use "as const", because that would
  // make the whole array readonly and cause type mismatches downstream
  return [searchParams, stableSetSearchParams] as ReturnType<
    typeof useSearchParams
  >;
}

const WorkspacesPage: FC = () => {
  const [dormantWorkspaces, setDormantWorkspaces] = useState<Workspace[]>([]);
  // If we use a useSearchParams for each hook, the values will not be in sync.
  // So we have to use a single one, centralizing the values, and pass it to
  // each hook.
  const searchParamsResult = useSafeSearchParams();
  const pagination = usePagination({ searchParamsResult });

  const organizationId = useOrganizationId();
  const templatesQuery = useQuery(templates(organizationId));

  const filterProps = useWorkspacesFilter({
    searchParamsResult,
    organizationId,
    onFilterChange: () => pagination.goToPage(1),
  });

  const { data, error, queryKey, refetch } = useWorkspacesData({
    ...pagination,
    query: filterProps.filter.query,
  });

  const experimentEnabled = useIsWorkspaceActionsEnabled();
  // If workspace actions are enabled we need to fetch the dormant
  // workspaces as well. This lets us determine whether we should
  // show a banner to the user indicating that some of their workspaces
  // are at risk of being deleted.
  useEffect(() => {
    if (experimentEnabled) {
      const includesDormant = filterProps.filter.query.includes("dormant_at");
      const dormantQuery = includesDormant
        ? filterProps.filter.query
        : filterProps.filter.query + " is-dormant:true";

      if (includesDormant && data) {
        setDormantWorkspaces(data.workspaces);
      } else {
        getWorkspaces({ q: dormantQuery })
          .then((resp) => {
            setDormantWorkspaces(resp.workspaces);
          })
          .catch(() => {
            // TODO
          });
      }
    } else {
      // If the experiment isn't included then we'll pretend
      // like dormant workspaces don't exist.
      setDormantWorkspaces([]);
    }
  }, [experimentEnabled, data, filterProps.filter.query]);
  const updateWorkspace = useWorkspaceUpdate(queryKey);
  const [checkedWorkspaces, setCheckedWorkspaces] = useState<Workspace[]>([]);
  const [isConfirmingDeleteAll, setIsConfirmingDeleteAll] = useState(false);
  const [urlSearchParams] = searchParamsResult;
  const { entitlements } = useDashboard();
  const canCheckWorkspaces =
    entitlements.features["workspace_batch_actions"].enabled;
  const permissions = usePermissions();
  const batchActions = useBatchActions({
    onSuccess: async () => {
      await refetch();
      setCheckedWorkspaces([]);
    },
  });

  // We want to uncheck the selected workspaces always when the url changes
  // because of filtering or pagination
  useEffect(() => {
    setCheckedWorkspaces([]);
  }, [urlSearchParams]);

  return (
    <>
      <Helmet>
        <title>{pageTitle("Workspaces")}</title>
      </Helmet>

      <WorkspacesPageView
        canCreateTemplate={permissions.createTemplates}
        checkedWorkspaces={checkedWorkspaces}
        onCheckChange={setCheckedWorkspaces}
        canCheckWorkspaces={canCheckWorkspaces}
        templates={templatesQuery.data}
        templatesFetchStatus={templatesQuery.status}
        workspaces={data?.workspaces}
        dormantWorkspaces={dormantWorkspaces}
        error={error}
        count={data?.count}
        page={pagination.page}
        limit={pagination.limit}
        onPageChange={pagination.goToPage}
        filterProps={filterProps}
        onUpdateWorkspace={(workspace) => {
          updateWorkspace.mutate(workspace);
        }}
        isRunningBatchAction={batchActions.isLoading}
        onDeleteAll={() => {
          setIsConfirmingDeleteAll(true);
        }}
        onStartAll={() => batchActions.startAll(checkedWorkspaces)}
        onStopAll={() => batchActions.stopAll(checkedWorkspaces)}
      />

      <BatchDeleteConfirmation
        isLoading={batchActions.isLoading}
        checkedWorkspaces={checkedWorkspaces}
        open={isConfirmingDeleteAll}
        onConfirm={async () => {
          await batchActions.deleteAll(checkedWorkspaces);
          setIsConfirmingDeleteAll(false);
        }}
        onClose={() => {
          setIsConfirmingDeleteAll(false);
        }}
      />
    </>
  );
};

export default WorkspacesPage;

type UseWorkspacesFilterOptions = {
  searchParamsResult: ReturnType<typeof useSearchParams>;
  onFilterChange: () => void;
  organizationId: string;
};

const useWorkspacesFilter = ({
  searchParamsResult,
  onFilterChange,
  organizationId,
}: UseWorkspacesFilterOptions) => {
  const filter = useFilter({
    fallbackFilter: "owner:me",
    searchParamsResult,
    onUpdate: onFilterChange,
  });

  const permissions = usePermissions();
  const canFilterByUser = permissions.viewDeploymentValues;
  const userMenu = useUserFilterMenu({
    value: filter.values.owner,
    onChange: (option) =>
      filter.update({ ...filter.values, owner: option?.value }),
    enabled: canFilterByUser,
  });

  const templateMenu = useTemplateFilterMenu({
    orgId: organizationId,
    value: filter.values.template,
    onChange: (option) =>
      filter.update({ ...filter.values, template: option?.value }),
  });

  const statusMenu = useStatusFilterMenu({
    value: filter.values.status,
    onChange: (option) =>
      filter.update({ ...filter.values, status: option?.value }),
  });

  return {
    filter,
    menus: {
      user: canFilterByUser ? userMenu : undefined,
      template: templateMenu,
      status: statusMenu,
    },
  };
};
