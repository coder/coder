import { paginatedInterceptions } from "api/queries/aiBridge";
import { useFilter } from "components/Filter/Filter";
import { useUserFilterMenu } from "components/Filter/UserFilter";
import { usePaginatedQuery } from "hooks/usePaginatedQuery";
import { useFeatureVisibility } from "modules/dashboard/useFeatureVisibility";
import type { FC } from "react";
import { useSearchParams } from "react-router";
import { pageTitle } from "utils/page";
import { useModelFilterMenu } from "./RequestLogsFilter/ModelFilter";
import { useProviderFilterMenu } from "./RequestLogsFilter/ProviderFilter";
import { RequestLogsPageView } from "./RequestLogsPageView";

const RequestLogsPage: FC = () => {
	const feats = useFeatureVisibility();
	const isRequestLogsVisible = Boolean(feats.aibridge);

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

	const providerMenu = useProviderFilterMenu({
		value: filter.values.provider,
		onChange: (option) =>
			filter.update({
				...filter.values,
				provider: option?.value,
			}),
	});

	const modelMenu = useModelFilterMenu({
		value: filter.values.model,
		onChange: (option) =>
			filter.update({
				...filter.values,
				model: option?.value,
			}),
	});

	return (
		<>
			<title>{pageTitle("Request Logs", "AI Bridge")}</title>

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
						provider: providerMenu,
						model: modelMenu,
					},
				}}
			/>
		</>
	);
};

export default RequestLogsPage;
