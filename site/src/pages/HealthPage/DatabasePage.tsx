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

const DatabasePage = () => {
	const healthStatus = useOutletContext<HealthcheckReport>();
	const database = healthStatus.database;

	return (
		<>
			<title>{pageTitle("Database - Health")}</title>

			<Header>
				<HeaderTitle>
					<HealthyDot severity={database.severity} />
					Database
				</HeaderTitle>
				<DismissWarningButton healthcheck="Database" />
			</Header>

			<Main>
				{database.warnings.map((warning) => {
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
					<GridDataLabel>Reachable</GridDataLabel>
					<GridDataValue>{database.reachable ? "Yes" : "No"}</GridDataValue>

					<GridDataLabel>Latency</GridDataLabel>
					<GridDataValue>{database.latency_ms}ms</GridDataValue>

					<GridDataLabel>Threshold</GridDataLabel>
					<GridDataValue>{database.threshold_ms}ms</GridDataValue>
				</GridData>
			</Main>
		</>
	);
};

export default DatabasePage;
