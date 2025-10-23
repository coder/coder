import { paginatedInterceptions } from "api/queries/aiBridge";
import { useFilter } from "components/Filter/Filter";
import { useUserFilterMenu } from "components/Filter/UserFilter";
import { usePaginatedQuery } from "hooks/usePaginatedQuery";
import { useFeatureVisibility } from "modules/dashboard/useFeatureVisibility";
import type { FC } from "react";
import { useSearchParams } from "react-router";
import { pageTitle } from "utils/page";
import { RequestLogsPageView } from "./RequestLogsPageView";

const RequestLogsPage: FC = () => {
	const feats = useFeatureVisibility();
	// The "else false" is required if aibridge is undefined.
	// It may happen if owner removes the license.
	//
	// see: https://github.com/coder/coder/issues/14798
	const isRequestLogsVisible = feats.aibridge || false;

	const [searchParams, setSearchParams] = useSearchParams();
	const interceptionsQuery = usePaginatedQuery(
		paginatedInterceptions(searchParams),
	);
	const filter = useFilter({
		searchParams,
		onSearchParamsChange: setSearchParams,
		onUpdate: interceptionsQuery.goToFirstPage,
	});

	const userMenu = useUserFilterMenu({
		value: filter.values.initiator,
		onChange: (option) =>
			filter.update({
				...filter.values,
				initiator: option?.value,
			}),
	});

	return (
		<>
			<title>{pageTitle("AI Governance", "Request Logs")}</title>

			<RequestLogsPageView
				isLoading={interceptionsQuery.isLoading}
				isRequestLogsVisible={isRequestLogsVisible}
				interceptions={interceptionsQuery.data?.results}
				interceptionsQuery={interceptionsQuery}
				filterProps={{
					filter,
					error: interceptionsQuery.error,
					menus: {
						user: userMenu,
					},
				}}
			/>
		</>
	);
};

export default RequestLogsPage;
