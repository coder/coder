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
import { deleteWorkspace, getWorkspaces } from "api/api";
import { ConfirmDialog } from "components/Dialogs/ConfirmDialog/ConfirmDialog";
import Box from "@mui/material/Box";
import { MONOSPACE_FONT_FAMILY } from "theme/constants";
import TextField from "@mui/material/TextField";
import { displayError } from "components/GlobalSnackbar/utils";
import { getErrorMessage } from "api/errors";
import { useEffectEvent } from "hooks/hookPolyfills";

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
  const filterProps = useWorkspacesFilter({
    searchParamsResult,
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
        : filterProps.filter.query + " dormant_at:1970-01-01";

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
  const [isDeletingAll, setIsDeletingAll] = useState(false);
  const [urlSearchParams] = searchParamsResult;
  const { entitlements } = useDashboard();
  const canCheckWorkspaces =
    entitlements.features["workspace_batch_actions"].enabled;

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
        checkedWorkspaces={checkedWorkspaces}
        onCheckChange={setCheckedWorkspaces}
        canCheckWorkspaces={canCheckWorkspaces}
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
        onDeleteAll={() => {
          setIsDeletingAll(true);
        }}
      />

      <BatchDeleteConfirmation
        checkedWorkspaces={checkedWorkspaces}
        open={isDeletingAll}
        onClose={() => {
          setIsDeletingAll(false);
        }}
        onDelete={async () => {
          await refetch();
          setCheckedWorkspaces([]);
        }}
      />
    </>
  );
};

export default WorkspacesPage;

type UseWorkspacesFilterOptions = {
  searchParamsResult: ReturnType<typeof useSearchParams>;
  onFilterChange: () => void;
};

const useWorkspacesFilter = ({
  searchParamsResult,
  onFilterChange,
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

  const orgId = useOrganizationId();
  const templateMenu = useTemplateFilterMenu({
    orgId,
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

const BatchDeleteConfirmation = ({
  checkedWorkspaces,
  open,
  onClose,
  onDelete,
}: {
  checkedWorkspaces: Workspace[];
  open: boolean;
  onClose: () => void;
  onDelete: () => void;
}) => {
  const [confirmValue, setConfirmValue] = useState("");
  const [confirmError, setConfirmError] = useState(false);
  const [isDeleting, setIsDeleting] = useState(false);

  const close = () => {
    if (isDeleting) {
      return;
    }

    onClose();
    setConfirmValue("");
    setConfirmError(false);
    setIsDeleting(false);
  };

  const confirmDeletion = async () => {
    setConfirmError(false);

    if (confirmValue !== "DELETE") {
      setConfirmError(true);
      return;
    }

    try {
      setIsDeleting(true);
      await Promise.all(checkedWorkspaces.map((w) => deleteWorkspace(w.id)));
    } catch (e) {
      displayError(
        "Error on deleting workspaces",
        getErrorMessage(e, "An error occurred while deleting the workspaces"),
      );
    } finally {
      close();
      onDelete();
    }
  };

  return (
    <ConfirmDialog
      type="delete"
      open={open}
      confirmLoading={isDeleting}
      onConfirm={confirmDeletion}
      onClose={() => {
        onClose();
        setConfirmValue("");
        setConfirmError(false);
      }}
      title={`Delete ${checkedWorkspaces?.length} ${
        checkedWorkspaces.length === 1 ? "workspace" : "workspaces"
      }`}
      description={
        <form
          onSubmit={async (e) => {
            e.preventDefault();
            await confirmDeletion();
          }}
        >
          <Box>
            Deleting these workspaces is irreversible! Are you sure you want to
            proceed? Type{" "}
            <Box
              component="code"
              sx={{
                fontFamily: MONOSPACE_FONT_FAMILY,
                color: (theme) => theme.palette.text.primary,
                fontWeight: 600,
              }}
            >
              `DELETE`
            </Box>{" "}
            to confirm.
          </Box>
          <TextField
            value={confirmValue}
            required
            autoFocus
            fullWidth
            inputProps={{
              "aria-label": "Type DELETE to confirm",
            }}
            placeholder="Type DELETE to confirm"
            sx={{ mt: 2 }}
            onChange={(e) => {
              setConfirmValue(e.currentTarget.value);
            }}
            error={confirmError}
            helperText={confirmError && "Please type DELETE to confirm"}
          />
        </form>
      }
    />
  );
};
