import type { FC } from "react";
import { useQueries } from "react-query";
import {
	customNotificationTemplates,
	notificationDispatchMethods,
	selectTemplatesByGroup,
	systemNotificationTemplates,
} from "#/api/queries/notifications";
import { Loader } from "#/components/Loader/Loader";
import {
	SettingsHeader,
	SettingsHeaderDescription,
	SettingsHeaderDocsLink,
	SettingsHeaderTitle,
} from "#/components/SettingsHeader/SettingsHeader";
import {
	Tabs,
	TabsContent,
	TabsList,
	TabsTrigger,
} from "#/components/Tabs/Tabs";
import { useSearchParamsKey } from "#/hooks/useSearchParamsKey";
import { useDeploymentConfig } from "#/modules/management/DeploymentConfigProvider";
import { castNotificationMethod } from "#/modules/notifications/utils";
import { deploymentGroupHasParent } from "#/utils/deployOptions";
import { docs } from "#/utils/docs";
import { pageTitle } from "#/utils/page";
import OptionsTable from "../OptionsTable";
import { NotificationEvents } from "./NotificationEvents";
import { Troubleshooting } from "./Troubleshooting";

const NOTIFICATION_TABS = ["events", "settings", "troubleshooting"] as const;

function isNotificationTab(
	value: string,
): value is (typeof NOTIFICATION_TABS)[number] {
	return (NOTIFICATION_TABS as readonly string[]).includes(value);
}

const NotificationsPage: FC = () => {
	const { deploymentConfig } = useDeploymentConfig();
	const [systemTemplatesByGroup, customTemplatesByGroup, dispatchMethods] =
		useQueries({
			queries: [
				{
					...systemNotificationTemplates(),
					select: selectTemplatesByGroup,
				},
				{
					...customNotificationTemplates(),
					select: selectTemplatesByGroup,
				},
				notificationDispatchMethods(),
			],
		});
	const tabState = useSearchParamsKey({
		key: "tab",
		defaultValue: "events",
	});

	const activeTab = isNotificationTab(tabState.value)
		? tabState.value
		: NOTIFICATION_TABS[0];

	const ready = !!(
		systemTemplatesByGroup.data &&
		customTemplatesByGroup.data &&
		dispatchMethods.data
	);
	// Combine system and custom notification templates
	const allTemplatesByGroup = {
		...systemTemplatesByGroup.data,
		...customTemplatesByGroup.data,
	};
	return (
		<>
			<title>{pageTitle("Notifications Settings")}</title>

			<SettingsHeader
				actions={
					<SettingsHeaderDocsLink
						href={docs("/admin/monitoring/notifications")}
					/>
				}
			>
				<SettingsHeaderTitle>Notifications</SettingsHeaderTitle>
				<SettingsHeaderDescription>
					Control delivery methods for notifications on this deployment.
				</SettingsHeaderDescription>
			</SettingsHeader>

			{!ready ? (
				<Loader />
			) : (
				<Tabs value={activeTab} onValueChange={tabState.setValue}>
					<TabsList>
						<TabsTrigger value="events">Events</TabsTrigger>
						<TabsTrigger value="settings">Settings</TabsTrigger>
						<TabsTrigger value="troubleshooting">Troubleshooting</TabsTrigger>
					</TabsList>
					<TabsContent value="events" className="py-6">
						<NotificationEvents
							templatesByGroup={allTemplatesByGroup}
							deploymentConfig={deploymentConfig.config}
							defaultMethod={castNotificationMethod(
								dispatchMethods.data.default,
							)}
							availableMethods={dispatchMethods.data.available.map(
								castNotificationMethod,
							)}
						/>
					</TabsContent>
					<TabsContent value="settings" className="py-6">
						<OptionsTable
							options={deploymentConfig.options.filter((o) =>
								deploymentGroupHasParent(o.group, "Notifications"),
							)}
						/>
					</TabsContent>
					<TabsContent value="troubleshooting" className="py-6">
						<Troubleshooting />
					</TabsContent>
				</Tabs>
			)}
		</>
	);
};

export default NotificationsPage;
