import { paginatedInterceptions } from "api/queries/aiBridge";
import { usePaginatedQuery } from "hooks/usePaginatedQuery";
import { useFeatureVisibility } from "modules/dashboard/useFeatureVisibility";
import type { FC } from "react";
import { useSearchParams } from "react-router";
import { pageTitle } from "utils/page";
import { RequestLogsPageView } from "./RequestLogsPageView";

const RequestLogsPage: FC = () => {
	const feats = useFeatureVisibility();
	// The "else false" is required if audit_log is undefined.
	// It may happen if owner removes the license.
	//
	// see: https://github.com/coder/coder/issues/14798
	const isRequestLogsVisible = feats.aibridge || false;

	const [searchParams, _setSearchParams] = useSearchParams();
	const interceptionsQuery = usePaginatedQuery(
		paginatedInterceptions(searchParams),
	);

	return (
		<>
			<title>{pageTitle("AI Governance", "Request Logs")}</title>

			<RequestLogsPageView
				isLoading={interceptionsQuery.isLoading}
				isRequestLogsVisible={isRequestLogsVisible}
				interceptions={interceptionsQuery.data?.results}
				interceptionsQuery={interceptionsQuery}
			/>
		</>
	);
};

export default RequestLogsPage;
