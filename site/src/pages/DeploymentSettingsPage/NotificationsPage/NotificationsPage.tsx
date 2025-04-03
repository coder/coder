import type { Interpolation, Theme } from "@emotion/react";
import {
	notificationDispatchMethods,
	selectTemplatesByGroup,
	systemNotificationTemplates,
} from "api/queries/notifications";
import { FeatureStageBadge } from "components/FeatureStageBadge/FeatureStageBadge";
import { Loader } from "components/Loader/Loader";
import {
	SettingsHeader,
	SettingsHeaderDescription,
	SettingsHeaderDocsLink,
	SettingsHeaderTitle,
} from "components/SettingsHeader/SettingsHeader";
import { TabLink, Tabs, TabsList } from "components/Tabs/Tabs";
import { useSearchParamsKey } from "hooks/useSearchParamsKey";
import { useDeploymentConfig } from "modules/management/DeploymentConfigProvider";
import { castNotificationMethod } from "modules/notifications/utils";
import type { FC } from "react";
import { Helmet } from "react-helmet-async";
import { useQueries } from "react-query";
import { deploymentGroupHasParent } from "utils/deployOptions";
import { docs } from "utils/docs";
import { pageTitle } from "utils/page";
import OptionsTable from "../OptionsTable";
import { NotificationEvents } from "./NotificationEvents";
import { Troubleshooting } from "./Troubleshooting";

export const NotificationsPage: FC = () => {
	const { deploymentConfig } = useDeploymentConfig();
	const [templatesByGroup, dispatchMethods] = useQueries({
		queries: [
			{
				...systemNotificationTemplates(),
				select: selectTemplatesByGroup,
			},
			notificationDispatchMethods(),
		],
	});
	const tabState = useSearchParamsKey({
		key: "tab",
		defaultValue: "events",
	});

	const ready = !!(templatesByGroup.data && dispatchMethods.data);
	return (
		<>
			<Helmet>
				<title>{pageTitle("Notifications Settings")}</title>
			</Helmet>

			<SettingsHeader
				actions={
					<SettingsHeaderDocsLink
						href={docs("/admin/monitoring/notifications")}
					/>
				}
			>
				<SettingsHeaderTitle>
					Notifications
				</SettingsHeaderTitle>
				<SettingsHeaderDescription>
					Control delivery methods for notifications on this deployment.
				</SettingsHeaderDescription>
			</SettingsHeader>

			<Tabs active={tabState.value}>
				<TabsList>
					<TabLink to="?tab=events" value="events">
						Events
					</TabLink>
					<TabLink to="?tab=settings" value="settings">
						Settings
					</TabLink>
					<TabLink to="?tab=troubleshooting" value="troubleshooting">
						Troubleshooting
					</TabLink>
				</TabsList>
			</Tabs>

			<div css={styles.content}>
				{ready ? (
					tabState.value === "events" ? (
						<NotificationEvents
							templatesByGroup={templatesByGroup.data}
							deploymentConfig={deploymentConfig.config}
							defaultMethod={castNotificationMethod(
								dispatchMethods.data.default,
							)}
							availableMethods={dispatchMethods.data.available.map(
								castNotificationMethod,
							)}
						/>
					) : tabState.value === "troubleshooting" ? (
						<Troubleshooting />
					) : (
						<OptionsTable
							options={deploymentConfig.options.filter((o) =>
								deploymentGroupHasParent(o.group, "Notifications"),
							)}
						/>
					)
				) : (
					<Loader />
				)}
			</div>
		</>
	);
};

export default NotificationsPage;

const styles = {
	content: { paddingTop: 24 },
} as Record<string, Interpolation<Theme>>;
