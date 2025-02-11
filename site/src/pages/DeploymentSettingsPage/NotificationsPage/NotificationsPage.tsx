import type { Interpolation, Theme } from "@emotion/react";
import {
	notificationDispatchMethods,
	selectTemplatesByGroup,
	sendTestNotification,
	systemNotificationTemplates,
} from "api/queries/notifications";
import { Loader } from "components/Loader/Loader";
import { TabLink, Tabs, TabsList } from "components/Tabs/Tabs";
import { useSearchParamsKey } from "hooks/useSearchParamsKey";
import { useDeploymentSettings } from "modules/management/DeploymentSettingsProvider";
import { castNotificationMethod } from "modules/notifications/utils";
import { Section } from "pages/UserSettingsPage/Section";
import type { FC } from "react";
import { Helmet } from "react-helmet-async";
import { useMutation, useQueries, useQueryClient } from "react-query";
import { deploymentGroupHasParent } from "utils/deployOptions";
import { pageTitle } from "utils/page";
import OptionsTable from "../OptionsTable";
import { NotificationEvents } from "./NotificationEvents";
import { Stack } from "components/Stack/Stack";
import { Button } from "components/Button/Button";

export const NotificationsPage: FC = () => {
	const { deploymentConfig } = useDeploymentSettings();
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
	const queryClient = useQueryClient();
	const sendNotification = useMutation(sendTestNotification(queryClient));

	const ready = !!(templatesByGroup.data && dispatchMethods.data);
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
				<Tabs active={tabState.value}>
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
						) : (
							<Stack>
								<Button
									className="w-fit"
									onClick={async () => {
										await sendNotification.mutateAsync();
									}}
								>
									Send test notification
								</Button>

								<OptionsTable
									options={deploymentConfig.options.filter((o) =>
										deploymentGroupHasParent(o.group, "Notifications"),
									)}
								/>
							</Stack>
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
