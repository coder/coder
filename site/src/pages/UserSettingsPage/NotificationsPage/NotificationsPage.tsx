import type { Interpolation, Theme } from "@emotion/react";
import Card from "@mui/material/Card";
import Divider from "@mui/material/Divider";
import List from "@mui/material/List";
import ListItem from "@mui/material/ListItem";
import ListItemIcon from "@mui/material/ListItemIcon";
import ListItemText, { listItemTextClasses } from "@mui/material/ListItemText";
import Switch from "@mui/material/Switch";
import {
	customNotificationTemplates,
	disableNotification,
	notificationDispatchMethods,
	selectTemplatesByGroup,
	systemNotificationTemplates,
	updateUserNotificationPreferences,
	userNotificationPreferences,
} from "api/queries/notifications";
import {
	preferenceSettings,
	updatePreferenceSettings,
} from "api/queries/users";
import type { NotificationTemplate } from "api/typesGenerated";
import { displayError, displaySuccess } from "components/GlobalSnackbar/utils";
import { Loader } from "components/Loader/Loader";
import { Stack } from "components/Stack/Stack";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "components/Tooltip/Tooltip";
import { useAuthenticated } from "hooks";
import {
	castNotificationMethod,
	isTaskNotification,
	methodIcons,
	methodLabels,
	notificationIsDisabled,
	selectDisabledPreferences,
} from "modules/notifications/utils";
import type { Permissions } from "modules/permissions";
import { type FC, Fragment, useEffect } from "react";
import { useMutation, useQueries, useQuery, useQueryClient } from "react-query";
import { useSearchParams } from "react-router";
import { pageTitle } from "utils/page";
import { Section } from "../Section";

const NotificationsPage: FC = () => {
	const { user, permissions } = useAuthenticated();
	const [
		disabledPreferences,
		systemTemplatesByGroup,
		customTemplatesByGroup,
		dispatchMethods,
	] = useQueries({
		queries: [
			{
				...userNotificationPreferences(user.id),
				select: selectDisabledPreferences,
			},
			{
				...systemNotificationTemplates(),
				select: (data: NotificationTemplate[]) => selectTemplatesByGroup(data),
			},
			{
				...customNotificationTemplates(),
				select: (data: NotificationTemplate[]) => selectTemplatesByGroup(data),
			},
			notificationDispatchMethods(),
		],
	});
	const queryClient = useQueryClient();
	const updatePreferences = useMutation(
		updateUserNotificationPreferences(user.id, queryClient),
	);

	// Notification emails contain a link to disable a specific notification
	// template. This functionality is achieved using the query string parameter
	// "disabled".
	const disableMutation = useMutation(
		disableNotification(user.id, queryClient),
	);
	const [searchParams] = useSearchParams();
	const disabledId = searchParams.get("disabled");
	useEffect(() => {
		if (!disabledId) {
			return;
		}
		searchParams.delete("disabled");
		disableMutation
			.mutateAsync(disabledId)
			.then(() => {
				displaySuccess("Notification has been disabled");
			})
			.catch(() => {
				displayError("Error disabling notification");
			});
	}, [searchParams.delete, disabledId, disableMutation]);

	const ready =
		disabledPreferences.data &&
		systemTemplatesByGroup.data &&
		customTemplatesByGroup.data &&
		dispatchMethods.data;
	// Combine system and custom notification templates
	const allTemplatesByGroup = {
		...systemTemplatesByGroup.data,
		...customTemplatesByGroup.data,
	};

	const preferencesQuery = useQuery(preferenceSettings());
	const updatePreferencesMutation = useMutation(
		updatePreferenceSettings(queryClient),
	);

	return (
		<>
			<title>{pageTitle("Notifications Settings")}</title>

			<Section
				title="Notifications"
				description="Control which notifications you receive."
				layout="fluid"
			>
				{ready ? (
					<Stack spacing={4}>
						{Object.entries(allTemplatesByGroup).map(([group, templates]) => {
							if (!canSeeNotificationGroup(group, permissions)) {
								return null;
							}

							const allDisabled = templates.some((tpl) => {
								return notificationIsDisabled(disabledPreferences.data, tpl);
							});

							return (
								<Card variant="outlined" className="bg-transparent" key={group}>
									<List>
										<ListItem
											css={(theme) => ({
												background: theme.palette.background.paper,
												borderBottom: `1px solid ${theme.palette.divider}`,
											})}
										>
											<ListItemIcon>
												<Switch
													id={group}
													size="small"
													checked={!allDisabled}
													onChange={async (_, checked) => {
														const updated = { ...disabledPreferences.data };
														for (const tpl of templates) {
															updated[tpl.id] = !checked;
														}
														await updatePreferences.mutateAsync({
															template_disabled_map: updated,
														});
														displaySuccess("Notification preferences updated");
													}}
												/>
											</ListItemIcon>
											<ListItemText
												css={{
													[`& .${listItemTextClasses.primary}`]: {
														fontSize: 14,
														fontWeight: 500,
														textTransform: "capitalize",
													},
													[`& .${listItemTextClasses.secondary}`]: {
														fontSize: 14,
													},
												}}
												primary={group}
												primaryTypographyProps={{
													component: "label",
													htmlFor: group,
												}}
											/>
										</ListItem>
										{templates.map((tmpl, i) => {
											const method = castNotificationMethod(
												tmpl.method || dispatchMethods.data.default,
											);
											const Icon = methodIcons[method];
											const label = methodLabels[method];
											const isLastItem = i === templates.length - 1;

											const disabled = notificationIsDisabled(
												disabledPreferences.data,
												tmpl,
											);

											return (
												<Fragment key={tmpl.id}>
													<ListItem>
														<ListItemIcon>
															<Switch
																size="small"
																id={tmpl.id}
																checked={!disabled}
																onChange={async (_, checked) => {
																	await updatePreferences.mutateAsync({
																		template_disabled_map: {
																			...disabledPreferences.data,
																			[tmpl.id]: !checked,
																		},
																	});

																	// Clear the Tasks page warning dismissal when enabling a task notification
																	// This ensures that if the user disables task notifications again later,
																	// they will see the warning alert again.
																	if (
																		isTaskNotification(tmpl) &&
																		checked &&
																		preferencesQuery.data
																	) {
																		updatePreferencesMutation.mutate({
																			task_notification_alert_dismissed: false,
																		});
																	}

																	displaySuccess(
																		"Notification preferences updated",
																	);
																}}
															/>
														</ListItemIcon>
														<ListItemText
															primaryTypographyProps={{
																component: "label",
																htmlFor: tmpl.id,
															}}
															css={{
																[`& .${listItemTextClasses.primary}`]: {
																	fontSize: 14,
																	fontWeight: 500,
																	textTransform: "capitalize",
																},
																[`& .${listItemTextClasses.secondary}`]: {
																	fontSize: 14,
																},
															}}
															primary={tmpl.name}
														/>
														<ListItemIcon
															css={(theme) => ({
																color: theme.palette.text.secondary,
																"& svg": {
																	fontSize: "inherit",
																},
															})}
															className="min-w-0 text-xl leading-none"
															aria-label="Delivery method"
														>
															<Tooltip>
																<TooltipTrigger asChild>
																	<Icon aria-label={label} />
																</TooltipTrigger>
																<TooltipContent side="bottom">
																	Delivery via {label}
																</TooltipContent>
															</Tooltip>
														</ListItemIcon>
													</ListItem>
													{!isLastItem && <Divider />}
												</Fragment>
											);
										})}
									</List>
								</Card>
							);
						})}
					</Stack>
				) : (
					<Loader />
				)}
			</Section>
		</>
	);
};

export default NotificationsPage;

function canSeeNotificationGroup(
	group: string,
	permissions: Permissions,
): boolean {
	switch (group) {
		case "Template Events":
			return permissions.createTemplates;
		case "User Events":
			return permissions.createUser;
		case "Workspace Events":
		case "Task Events":
		case "Custom Events":
			return true;
		default:
			return false;
	}
}
