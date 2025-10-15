import { listInterceptions } from "api/queries/aiBridge";
import type { FC } from "react";
import { useQuery } from "react-query";
import { pageTitle } from "utils/page";
import { RequestLogsPageView } from "./RequestLogsPageView";

const RequestLogsPage: FC = () => {
	const interceptionsQuery = useQuery(listInterceptions());

	return (
		<>
			<title>{pageTitle("AI Governance", "Request Logs")}</title>

			<RequestLogsPageView
				isLoading={interceptionsQuery.isLoading}
				interceptions={interceptionsQuery.data?.results}
			/>
		</>
	);
};

export default RequestLogsPage;
