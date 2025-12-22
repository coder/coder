import type { HealthcheckReport } from "api/typesGenerated";
import { Alert } from "components/Alert/Alert";
import { useOutletContext } from "react-router";
import { pageTitle } from "utils/page";
import {
	GridData,
	GridDataLabel,
	GridDataValue,
	Header,
	HeaderTitle,
	HealthMessageDocsLink,
	HealthyDot,
	Main,
} from "./Content";
import { DismissWarningButton } from "./DismissWarningButton";

const AccessURLPage = () => {
	const healthStatus = useOutletContext<HealthcheckReport>();
	const accessUrl = healthStatus.access_url;

	return (
		<>
			<title>{pageTitle("Access URL - Health")}</title>

			<Header>
				<HeaderTitle>
					<HealthyDot severity={accessUrl.severity} />
					Access URL
				</HeaderTitle>
				<DismissWarningButton healthcheck="AccessURL" />
			</Header>

			<Main>
				{accessUrl.error && <Alert severity="error">{accessUrl.error}</Alert>}

				{accessUrl.warnings.map((warning) => {
					return (
						<Alert
							actions={<HealthMessageDocsLink {...warning} />}
							key={warning.code}
							severity="warning"
							prominent
						>
							{warning.message}
						</Alert>
					);
				})}

				<GridData>
					<GridDataLabel>Severity</GridDataLabel>
					<GridDataValue>{accessUrl.severity}</GridDataValue>

					<GridDataLabel>Access URL</GridDataLabel>
					<GridDataValue>{accessUrl.access_url}</GridDataValue>

					<GridDataLabel>Reachable</GridDataLabel>
					<GridDataValue>{accessUrl.reachable ? "Yes" : "No"}</GridDataValue>

					<GridDataLabel>Status Code</GridDataLabel>
					<GridDataValue>{accessUrl.status_code}</GridDataValue>
				</GridData>
			</Main>
		</>
	);
};

export default AccessURLPage;
