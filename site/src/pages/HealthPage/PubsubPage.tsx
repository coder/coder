import { useOutletContext } from "react-router";
import type { HealthcheckReport } from "#/api/typesGenerated";
import { Alert } from "#/components/Alert/Alert";
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

const PubsubPage: React.FC = () => {
	const healthStatus = useOutletContext<HealthcheckReport>();
	const { pubsub } = healthStatus;

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
							// The NATS backend can report the same code for several connections.
							key={`${warning.code}-${warning.message}`}
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
				</GridData>
			</Main>
		</>
	);
};

export default PubsubPage;
