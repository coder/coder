import type { Interpolation, Theme } from "@emotion/react";
import {
	notificationDispatchMethods,
	selectTemplatesByGroup,
	systemNotificationTemplates,
} from "api/queries/notifications";
import { Loader } from "components/Loader/Loader";
import { TabLink, Tabs, TabsList } from "components/Tabs/Tabs";
import { useManagementSettings } from "modules/management/ManagementSettingsLayout";
import { castNotificationMethod } from "modules/notifications/utils";
import { Section } from "pages/UserSettingsPage/Section";
import type { FC } from "react";
import { Helmet } from "react-helmet-async";
import { useQueries } from "react-query";
import { useSearchParams } from "react-router-dom";
import { deploymentGroupHasParent } from "utils/deployOptions";
import { pageTitle } from "utils/page";
import OptionsTable from "../OptionsTable";
import { NotificationEvents } from "./NotificationEvents";

export const NotificationsPage: FC = () => {
	const [searchParams] = useSearchParams();
	const { deploymentValues } = useManagementSettings();
	const [templatesByGroup, dispatchMethods] = useQueries({
		queries: [
			{
				...systemNotificationTemplates(),
				select: selectTemplatesByGroup,
			},
			notificationDispatchMethods(),
		],
	});
	const ready =
		templatesByGroup.data && dispatchMethods.data && deploymentValues;
	const tab = searchParams.get("tab") || "events";

	return (
		<>
			<Helmet>
				<title>{pageTitle("Notifications Settings")}</title>
			</Helmet>
			<Section
				title="Notifications"
				description="Control delivery methods for notifications on this deployment."
				layout="fluid"
				featureStage={"beta"}
			>
				<Tabs active={tab}>
					<TabsList>
						<TabLink to="?tab=events" value="events">
							Events
						</TabLink>
						<TabLink to="?tab=settings" value="settings">
							Settings
						</TabLink>
					</TabsList>
				</Tabs>

				<div css={styles.content}>
					{ready ? (
						tab === "events" ? (
							<NotificationEvents
								templatesByGroup={templatesByGroup.data}
								deploymentValues={deploymentValues.config}
								defaultMethod={castNotificationMethod(
									dispatchMethods.data.default,
								)}
								availableMethods={dispatchMethods.data.available.map(
									castNotificationMethod,
								)}
							/>
						) : (
							<OptionsTable
								options={deploymentValues?.options.filter((o) =>
									deploymentGroupHasParent(o.group, "Notifications"),
								)}
							/>
						)
					) : (
						<Loader />
					)}
				</div>
			</Section>
		</>
	);
};

export default NotificationsPage;

const styles = {
	content: { paddingTop: 24 },
} as Record<string, Interpolation<Theme>>;
