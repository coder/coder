import type { Interpolation, Theme } from "@emotion/react";
import Button from "@mui/material/Button";
import Card from "@mui/material/Card";
import Divider from "@mui/material/Divider";
import List from "@mui/material/List";
import ListItem from "@mui/material/ListItem";
import ListItemText, { listItemTextClasses } from "@mui/material/ListItemText";
import ToggleButton from "@mui/material/ToggleButton";
import ToggleButtonGroup from "@mui/material/ToggleButtonGroup";
import Tooltip from "@mui/material/Tooltip";
import { getErrorMessage } from "api/errors";
import {
	type selectTemplatesByGroup,
	updateNotificationTemplateMethod,
} from "api/queries/notifications";
import type { DeploymentValues } from "api/typesGenerated";
import { Alert } from "components/Alert/Alert";
import { displayError, displaySuccess } from "components/GlobalSnackbar/utils";
import { Stack } from "components/Stack/Stack";
import {
	type NotificationMethod,
	castNotificationMethod,
	methodIcons,
	methodLabels,
} from "modules/notifications/utils";
import { type FC, Fragment } from "react";
import { useMutation, useQueryClient } from "react-query";
import { docs } from "utils/docs";

type NotificationEventsProps = {
	defaultMethod: NotificationMethod;
	availableMethods: NotificationMethod[];
	templatesByGroup: ReturnType<typeof selectTemplatesByGroup>;
	deploymentConfig: DeploymentValues;
};

export const NotificationEvents: FC<NotificationEventsProps> = ({
	defaultMethod,
	availableMethods,
	templatesByGroup,
	deploymentConfig,
}) => {
	// Webhook
	const hasWebhookNotifications = Object.values(templatesByGroup)
		.flat()
		.some((t) => t.method === "webhook");
	const webhookValues = deploymentConfig.notifications?.webhook ?? {};
	const isWebhookConfigured = requiredFieldsArePresent(webhookValues, [
		"endpoint",
	]);

	// SMTP
	const hasSMTPNotifications = Object.values(templatesByGroup)
		.flat()
		.some((t) => t.method === "smtp");
	const smtpValues = deploymentConfig.notifications?.email ?? {};
	const isSMTPConfigured = requiredFieldsArePresent(smtpValues, [
		"smarthost",
		"from",
		"hello",
	]);

	return (
		<Stack spacing={4}>
			{hasWebhookNotifications && !isWebhookConfigured && (
				<Alert
					severity="warning"
					actions={
						<Button
							variant="text"
							size="small"
							component="a"
							target="_blank"
							rel="noreferrer"
							href={docs("/admin/monitoring/notifications#webhook")}
						>
							Read the docs
						</Button>
					}
				>
					Webhook notifications are enabled, but not properly configured.
				</Alert>
			)}

			{hasSMTPNotifications && !isSMTPConfigured && (
				<Alert
					severity="warning"
					actions={
						<Button
							variant="text"
							size="small"
							component="a"
							target="_blank"
							rel="noreferrer"
							href={docs("/admin/monitoring/notifications#smtp-email")}
						>
							Read the docs
						</Button>
					}
				>
					SMTP notifications are enabled but not properly configured.
				</Alert>
			)}

			{Object.entries(templatesByGroup).map(([group, templates]) => (
				<Card
					key={group}
					variant="outlined"
					css={{ background: "transparent", width: "100%" }}
				>
					<List>
						<ListItem css={styles.listHeader}>
							<ListItemText css={styles.listItemText} primary={group} />
						</ListItem>

						{templates.map((tpl, i) => {
							const value = castNotificationMethod(tpl.method || defaultMethod);
							const isLastItem = i === templates.length - 1;

							return (
								<Fragment key={tpl.id}>
									<ListItem>
										<ListItemText
											css={styles.listItemText}
											primary={tpl.name}
										/>
										<MethodToggleGroup
											templateId={tpl.id}
											options={availableMethods}
											value={value}
										/>
									</ListItem>
									{!isLastItem && <Divider />}
								</Fragment>
							);
						})}
					</List>
				</Card>
			))}
		</Stack>
	);
};

function requiredFieldsArePresent(
	obj: Record<string, string | undefined>,
	fields: string[],
): boolean {
	return fields.every((field) => Boolean(obj[field]));
}

type MethodToggleGroupProps = {
	templateId: string;
	options: NotificationMethod[];
	value: NotificationMethod;
};

const MethodToggleGroup: FC<MethodToggleGroupProps> = ({
	value,
	options,
	templateId,
}) => {
	const queryClient = useQueryClient();
	const updateMethodMutation = useMutation(
		updateNotificationTemplateMethod(templateId, queryClient),
	);

	return (
		<ToggleButtonGroup
			exclusive
			value={value}
			size="small"
			aria-label="Notification method"
			css={styles.toggleGroup}
			onChange={async (_, method) => {
				try {
					await updateMethodMutation.mutateAsync({
						method,
					});
					displaySuccess("Notification method updated");
				} catch (error) {
					displayError(
						getErrorMessage(error, "Failed to update notification method"),
					);
				}
			}}
		>
			{options.map((method) => {
				const Icon = methodIcons[method];
				const label = methodLabels[method];
				return (
					<Tooltip key={method} title={label}>
						<ToggleButton
							value={method}
							css={styles.toggleButton}
							onClick={(e) => {
								// Retain the value if the user clicks the same button, ensuring
								// at least one value remains selected.
								if (method === value) {
									e.preventDefault();
									e.stopPropagation();
									return;
								}
							}}
						>
							<Icon aria-label={label} />
						</ToggleButton>
					</Tooltip>
				);
			})}
		</ToggleButtonGroup>
	);
};

const styles = {
	listHeader: (theme) => ({
		background: theme.palette.background.paper,
		borderBottom: `1px solid ${theme.palette.divider}`,
	}),
	listItemText: {
		[`& .${listItemTextClasses.primary}`]: {
			fontSize: 14,
			fontWeight: 500,
		},
		[`& .${listItemTextClasses.secondary}`]: {
			fontSize: 14,
		},
	},
	toggleGroup: (theme) => ({
		border: `1px solid ${theme.palette.divider}`,
		borderRadius: 4,
	}),
	toggleButton: (theme) => ({
		border: 0,
		borderRadius: 4,
		fontSize: 16,
		padding: "4px 8px",
		color: theme.palette.text.disabled,

		"&:hover": {
			color: theme.palette.text.primary,
		},

		"& svg": {
			fontSize: "inherit",
		},
	}),
} as Record<string, Interpolation<Theme>>;
