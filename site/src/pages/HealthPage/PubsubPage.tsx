import { useOutletContext } from "react-router";
import type { HealthcheckReport } from "#/api/typesGenerated";
import { Alert } from "#/components/Alert/Alert";
import { pageTitle } from "#/utils/page";
import { formatDateTime } from "#/utils/time";
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

const PubsubPage = () => {
	const healthStatus = useOutletContext<HealthcheckReport>();
	const pubsub = healthStatus.pubsub;

	return (
		<>
			<title>{pageTitle("Pubsub - Health")}</title>

			<Header>
				<HeaderTitle>
					<HealthyDot severity={pubsub.severity} />
					Pubsub
				</HeaderTitle>
				<MuteWarningsButton healthcheck="Pubsub" />
			</Header>

			<Main>
				{pubsub.error && (
					<Alert severity="error" prominent>
						{pubsub.error}
					</Alert>
				)}
				{pubsub.warnings.map((warning) => {
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
					<GridDataLabel>Backend</GridDataLabel>
					<GridDataValue>{pubsub.backend || "Unknown"}</GridDataValue>

					<GridDataLabel>Connected</GridDataLabel>
					<GridDataValue>
						{pubsub.connected == null
							? "Unknown"
							: pubsub.connected
								? "Yes"
								: "No"}
					</GridDataValue>

					<GridDataLabel>Last connection state change</GridDataLabel>
					<GridDataValue>
						{pubsub.last_connection_state_change
							? formatDateTime(pubsub.last_connection_state_change)
							: "Unknown"}
					</GridDataValue>
				</GridData>
			</Main>
		</>
	);
};

export default PubsubPage;
