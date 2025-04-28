import type { Interpolation, Theme } from "@emotion/react";
import Card from "@mui/material/Card";
import Divider from "@mui/material/Divider";
import List from "@mui/material/List";
import ListItem from "@mui/material/ListItem";
import ListItemIcon from "@mui/material/ListItemIcon";
import ListItemText, { listItemTextClasses } from "@mui/material/ListItemText";
import Switch from "@mui/material/Switch";
import Tooltip from "@mui/material/Tooltip";
import {
	disableNotification,
	notificationDispatchMethods,
	selectTemplatesByGroup,
	systemNotificationTemplates,
	updateUserNotificationPreferences,
	userNotificationPreferences,
} from "api/queries/notifications";
import type {
	NotificationPreference,
	NotificationTemplate,
} from "api/typesGenerated";
import { displayError, displaySuccess } from "components/GlobalSnackbar/utils";
import { Loader } from "components/Loader/Loader";
import { Stack } from "components/Stack/Stack";
import { useAuthenticated } from "hooks";
import {
	castNotificationMethod,
	methodIcons,
	methodLabels,
} from "modules/notifications/utils";
import { type FC, Fragment } from "react";
import { useEffect } from "react";
import { Helmet } from "react-helmet-async";
import { useMutation, useQueries, useQueryClient } from "react-query";
import { useSearchParams } from "react-router-dom";
import { pageTitle } from "utils/page";
import { Section } from "../Section";

export const NotificationsPage: FC = () => {
	const { user, permissions } = useAuthenticated();
	const [disabledPreferences, templatesByGroup, dispatchMethods] = useQueries({
		queries: [
			{
				...userNotificationPreferences(user.id),
				select: selectDisabledPreferences,
			},
			{
				...systemNotificationTemplates(),
				select: (data: NotificationTemplate[]) => {
					const groups = selectTemplatesByGroup(data);
					return permissions.viewDeploymentConfig
						? groups
						: {
								// Members only have access to the "Workspace Notifications" group
								"Workspace Events": groups["Workspace Events"],
							};
				},
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
		disabledPreferences.data && templatesByGroup.data && dispatchMethods.data;

	return (
		<>
			<Helmet>
				<title>{pageTitle("Notifications Settings")}</title>
			</Helmet>
			<Section
				title="Notifications"
				description="Control which notifications you receive."
				layout="fluid"
			>
				{ready ? (
					<Stack spacing={4}>
						{Object.entries(templatesByGroup.data).map(([group, templates]) => {
							const allDisabled = templates.some((tpl) => {
								return notificationIsDisabled(disabledPreferences.data, tpl);
							});

							return (
								<Card
									variant="outlined"
									css={{ background: "transparent" }}
									key={group}
								>
									<List>
										<ListItem css={styles.listHeader}>
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
												css={styles.listItemText}
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
															css={styles.listItemText}
															primary={tmpl.name}
														/>
														<ListItemIcon
															css={styles.listItemEndIcon}
															aria-label="Delivery method"
														>
															<Tooltip title={`Delivery via ${label}`}>
																<Icon aria-label={label} />
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

function notificationIsDisabled(
	disabledPreferences: Record<string, boolean>,
	tmpl: NotificationTemplate,
): boolean {
	return (
		(!tmpl.enabled_by_default && disabledPreferences[tmpl.id] === undefined) ||
		!!disabledPreferences[tmpl.id]
	);
}

function selectDisabledPreferences(data: NotificationPreference[]) {
	return data.reduce(
		(acc, pref) => {
			acc[pref.id] = pref.disabled;
			return acc;
		},
		{} as Record<string, boolean>,
	);
}

const styles = {
	listHeader: (theme) => ({
		background: theme.palette.background.paper,
		borderBottom: `1px solid ${theme.palette.divider}`,
	}),
	listItemText: {
		[`& .${listItemTextClasses.primary}`]: {
			fontSize: 14,
			fontWeight: 500,
			textTransform: "capitalize",
		},
		[`& .${listItemTextClasses.secondary}`]: {
			fontSize: 14,
		},
	},
	listItemEndIcon: (theme) => ({
		minWidth: 0,
		fontSize: 20,
		color: theme.palette.text.secondary,

		"& svg": {
			fontSize: "inherit",
		},
	}),
} as Record<string, Interpolation<Theme>>;
