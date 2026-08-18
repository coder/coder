import { useOutletContext } from "react-router";
import type { HealthcheckReport } from "#/api/typesGenerated";
import { Alert } from "#/components/Alert/Alert";
import { Loader } from "#/components/Loader/Loader";
import { createDayString } from "#/utils/createDayString";
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

const UsagePublishingPage = () => {
	const healthStatus = useOutletContext<HealthcheckReport>();
	const usagePublishing = healthStatus.usage_publishing;

	// Older replicas do not report this section, e.g. while a rolling upgrade
	// serves this frontend alongside an older coderd. The health query
	// refetches periodically, so show a loader until the upgraded response
	// arrives.
	if (!usagePublishing) {
		return <Loader />;
	}

	return (
		<>
			<title>{pageTitle("Usage Publishing - Health")}</title>

			<Header>
				<HeaderTitle>
					<HealthyDot severity={usagePublishing.severity} />
					Usage Publishing
				</HeaderTitle>
				<MuteWarningsButton healthcheck="UsagePublishing" />
			</Header>

			<Main>
				{usagePublishing.warnings.map((warning) => {
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
					<GridDataLabel>Publishing enabled</GridDataLabel>
					<GridDataValue>
						{usagePublishing.publishing_enabled ? "Yes" : "No"}
					</GridDataValue>

					<GridDataLabel>Last published</GridDataLabel>
					<GridDataValue>
						{usagePublishing.last_published_at
							? createDayString(usagePublishing.last_published_at)
							: "Never"}
					</GridDataValue>

					{usagePublishing.failing_since && (
						<>
							<GridDataLabel>Failing since</GridDataLabel>
							<GridDataValue>
								{createDayString(usagePublishing.failing_since)}
							</GridDataValue>
						</>
					)}
				</GridData>
			</Main>
		</>
	);
};

export default UsagePublishingPage;
