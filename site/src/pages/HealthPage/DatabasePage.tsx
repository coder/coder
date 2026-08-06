import { Alert } from "#/components/Alert/Alert";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Loader } from "#/components/Loader/Loader";
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
import { useHealthStatus } from "./useHealthStatus";

const DatabasePage = () => {
	const { data: healthStatus, isLoading, error } = useHealthStatus();

	if (isLoading) {
		return <Loader />;
	}

	if (error || !healthStatus) {
		return <ErrorAlert error={error} />;
	}

	const database = healthStatus.database;

	return (
		<>
			<title>{pageTitle("Database - Health")}</title>

			<Header>
				<HeaderTitle>
					<HealthyDot severity={database.severity} />
					Database
				</HeaderTitle>
				<MuteWarningsButton healthcheck="Database" />
			</Header>

			<Main>
				{database.warnings.map((warning) => {
					return (
						<Alert
							actions={<HealthMessageDocsLink {...warning} />}
							key={warning.code}
							severity="warning"
							prominent
							dismissible
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
