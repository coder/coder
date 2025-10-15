import { listInterceptions } from "api/queries/aiBridge";
import { useFeatureVisibility } from "modules/dashboard/useFeatureVisibility";
import type { FC } from "react";
import { useQuery } from "react-query";
import { pageTitle } from "utils/page";
import { RequestLogsPageView } from "./RequestLogsPageView";

const RequestLogsPage: FC = () => {
	const feats = useFeatureVisibility();
	// The "else false" is required if audit_log is undefined.
	// It may happen if owner removes the license.
	//
	// see: https://github.com/coder/coder/issues/14798
	const isRequestLogsVisible = feats.aibridge || false;

	const interceptionsQuery = useQuery(listInterceptions());

	return (
		<>
			<title>{pageTitle("AI Governance", "Request Logs")}</title>

			<RequestLogsPageView
				isLoading={interceptionsQuery.isLoading}
				isRequestLogsVisible={isRequestLogsVisible}
				interceptions={interceptionsQuery.data?.results}
			/>
		</>
	);
};

export default RequestLogsPage;
