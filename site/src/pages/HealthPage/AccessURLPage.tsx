import { useOutletContext } from "react-router";
import type { HealthcheckReport } from "#/api/typesGenerated";
import { Alert, AlertDescription, AlertTitle } from "#/components/Alert/Alert";
import { pageTitle } from "#/utils/page";
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
import { MuteWarningsButton } from "./MuteWarningsButton";

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
				<MuteWarningsButton healthcheck="AccessURL" />
			</Header>

			<Main>
				{accessUrl.error && (
					<Alert severity="error" prominent>
						<AlertTitle>The access URL check failed</AlertTitle>
						<AlertDescription>{accessUrl.error}</AlertDescription>
					</Alert>
				)}

				{accessUrl.warnings.map((warning) => {
					return (
						<Alert
							actions={<HealthMessageDocsLink {...warning} />}
							key={warning.code}
							severity="warning"
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
