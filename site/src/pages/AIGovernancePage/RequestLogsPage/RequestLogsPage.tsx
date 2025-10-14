import type { FC } from "react";
import { pageTitle } from "utils/page";
import { RequestLogsPageView } from "./RequestLogsPageView";

const RequestLogsPage: FC = () => {
	return (
		<>
			<title>{pageTitle("AI Governance", "Request Logs")}</title>

			<RequestLogsPageView />
		</>
	)
};

export default RequestLogsPage;
