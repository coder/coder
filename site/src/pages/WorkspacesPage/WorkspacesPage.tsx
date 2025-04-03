import { workspacePermissionsByOrganization } from "api/queries/organizations";
import { templates } from "api/queries/templates";
import type { Workspace } from "api/typesGenerated";
import { useFilter } from "components/Filter/Filter";
import { useUserFilterMenu } from "components/Filter/UserFilter";
import { useAuthenticated } from "contexts/auth/RequireAuth";
import { useEffectEvent } from "hooks/hookPolyfills";
import { usePagination } from "hooks/usePagination";
import { useDashboard } from "modules/dashboard/useDashboard";
import { useOrganizationsFilterMenu } from "modules/tableFiltering/options";
import { type FC, useEffect, useMemo, useState } from "react";
import { Helmet } from "react-helmet-async";
import { useQuery } from "react-query";
import { useSearchParams } from "react-router-dom";
import { pageTitle } from "utils/page";
import { BatchDeleteConfirmation } from "./BatchDeleteConfirmation";
import { BatchUpdateConfirmation } from "./BatchUpdateConfirmation";
import { WorkspacesPageView } from "./WorkspacesPageView";
import { useBatchActions } from "./batchActions";
import { useWorkspaceUpdate, useWorkspacesData } from "./data";
import { useStatusFilterMenu, useTemplateFilterMenu } from "./filter/menus";

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
	// If we use a useSearchParams for each hook, the values will not be in sync.
	// So we have to use a single one, centralizing the values, and pass it to
	// each hook.
	const searchParamsResult = useSafeSearchParams();
	const pagination = usePagination({ searchParamsResult });
	const { permissions, user: me } = useAuthenticated();
	const { entitlements } = useDashboard();

	const templatesQuery = useQuery(templates());

	const orgPermissionsQuery = useQuery(
		workspacePermissionsByOrganization(
			templatesQuery.data?.map((template) => template.organization_id),
			me.id,
		),
	);

	// Filter templates based on workspace creation permission
	const filteredTemplates = useMemo(() => {
		if (!templatesQuery.data || !orgPermissionsQuery.data) {
			return templatesQuery.data;
		}

		return templatesQuery.data.filter((template) => {
			const orgPermission = orgPermissionsQuery.data[template.organization_id];
			return orgPermission?.createWorkspace;
		});
	}, [templatesQuery.data, orgPermissionsQuery.data]);

	const filterProps = useWorkspacesFilter({
		searchParamsResult,
		onFilterChange: () => pagination.goToPage(1),
	});

	const { data, error, queryKey, refetch } = useWorkspacesData({
		...pagination,
		query: filterProps.filter.query,
	});

	const updateWorkspace = useWorkspaceUpdate(queryKey);
	const [checkedWorkspaces, setCheckedWorkspaces] = useState<
		readonly Workspace[]
	>([]);
	const [confirmingBatchAction, setConfirmingBatchAction] = useState<
		"delete" | "update" | null
	>(null);
	const [urlSearchParams] = searchParamsResult;
	const canCheckWorkspaces =
		entitlements.features.workspace_batch_actions.enabled;
	const batchActions = useBatchActions({
		onSuccess: async () => {
			await refetch();
			setCheckedWorkspaces([]);
		},
	});

	// We want to uncheck the selected workspaces always when the url changes
	// because of filtering or pagination
	// biome-ignore lint/correctness/useExhaustiveDependencies: consider refactoring
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
				canChangeVersions={permissions.updateTemplates}
				checkedWorkspaces={checkedWorkspaces}
				onCheckChange={setCheckedWorkspaces}
				canCheckWorkspaces={canCheckWorkspaces}
				templates={filteredTemplates}
				templatesFetchStatus={templatesQuery.status}
				workspaces={data?.workspaces}
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
				onDeleteAll={() => setConfirmingBatchAction("delete")}
				onUpdateAll={() => setConfirmingBatchAction("update")}
				onStartAll={() => batchActions.startAll(checkedWorkspaces)}
				onStopAll={() => batchActions.stopAll(checkedWorkspaces)}
			/>

			<BatchDeleteConfirmation
				isLoading={batchActions.isLoading}
				checkedWorkspaces={checkedWorkspaces}
				open={confirmingBatchAction === "delete"}
				onConfirm={async () => {
					await batchActions.deleteAll(checkedWorkspaces);
					setConfirmingBatchAction(null);
				}}
				onClose={() => {
					setConfirmingBatchAction(null);
				}}
			/>

			<BatchUpdateConfirmation
				isLoading={batchActions.isLoading}
				checkedWorkspaces={checkedWorkspaces}
				open={confirmingBatchAction === "update"}
				onConfirm={async () => {
					await batchActions.updateAll(checkedWorkspaces);
					setConfirmingBatchAction(null);
				}}
				onClose={() => {
					setConfirmingBatchAction(null);
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

	const { permissions } = useAuthenticated();
	const canFilterByUser = permissions.viewDeploymentConfig;
	const userMenu = useUserFilterMenu({
		value: filter.values.owner,
		onChange: (option) =>
			filter.update({ ...filter.values, owner: option?.value }),
		enabled: canFilterByUser,
	});

	const templateMenu = useTemplateFilterMenu({
		value: filter.values.template,
		onChange: (option) =>
			filter.update({ ...filter.values, template: option?.value }),
	});

	const statusMenu = useStatusFilterMenu({
		value: filter.values.status,
		onChange: (option) =>
			filter.update({ ...filter.values, status: option?.value }),
	});

	const { showOrganizations } = useDashboard();
	const organizationsMenu = useOrganizationsFilterMenu({
		value: filter.values.organization,
		onChange: (option) => {
			filter.update({
				...filter.values,
				organization: option?.value,
			});
		},
	});

	return {
		filter,
		menus: {
			user: canFilterByUser ? userMenu : undefined,
			template: templateMenu,
			status: statusMenu,
			organizations: showOrganizations ? organizationsMenu : undefined,
		},
	};
};
