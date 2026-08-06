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

const AccessURLPage = () => {
	const { data: healthStatus, isLoading, error } = useHealthStatus();

	if (isLoading) {
		return <Loader />;
	}

	if (error || !healthStatus) {
		return <ErrorAlert error={error} />;
	}

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
				{accessUrl.error && <Alert severity="error">{accessUrl.error}</Alert>}

				{accessUrl.warnings.map((warning) => {
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
