import { useSearchParams } from "react-router";
import { paginatedConnectionLogs } from "#/api/queries/connectionlog";
import { useFilter } from "#/components/Filter/Filter";
import { isNonInitialPage } from "#/components/PaginationWidget/utils";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { usePaginatedQuery } from "#/hooks/usePaginatedQuery";
import { useFeatureVisibility } from "#/modules/dashboard/useFeatureVisibility";
import { pageTitle } from "#/utils/page";
import { ConnectionLogPageView } from "./ConnectionLogPageView";

const ConnectionLogPage: React.FC = () => {
	const { permissions } = useAuthenticated();
	const feats = useFeatureVisibility();

	// The "else false" is required if connection_log is undefined, which may
	// happen if the license is removed.
	//
	// see: https://github.com/coder/coder/issues/14798
	const isConnectionLogVisible = feats.connection_log || false;

	const [searchParams, setSearchParams] = useSearchParams();
	const connectionlogsQuery = usePaginatedQuery(
		paginatedConnectionLogs(searchParams),
	);
	const filter = useFilter({
		searchParams,
		onSearchParamsChange: setSearchParams,
		onUpdate: connectionlogsQuery.goToFirstPage,
	});

	return (
		<>
			<title>{pageTitle("Connection Log")}</title>

			<ConnectionLogPageView
				connectionLogs={connectionlogsQuery.data?.connection_logs}
				isNonInitialPage={isNonInitialPage(searchParams)}
				isConnectionLogVisible={isConnectionLogVisible}
				connectionLogsQuery={connectionlogsQuery}
				error={connectionlogsQuery.error}
				permissions={permissions}
				filterProps={{
					filter,
					error: connectionlogsQuery.error,
				}}
			/>
		</>
	);
};

export default ConnectionLogPage;
