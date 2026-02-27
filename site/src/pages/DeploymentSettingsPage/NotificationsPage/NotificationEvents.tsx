import { getErrorDetail, getErrorMessage } from "api/errors";
import {
	type selectTemplatesByGroup,
	updateNotificationTemplateMethod,
} from "api/queries/notifications";
import type { DeploymentValues } from "api/typesGenerated";
import { Alert } from "components/Alert/Alert";
import { Button } from "components/Button/Button";
import {
	ToggleGroup,
	ToggleGroupItem,
} from "components/ToggleGroup/ToggleGroup";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "components/Tooltip/Tooltip";
import {
	castNotificationMethod,
	methodIcons,
	methodLabels,
	type NotificationMethod,
} from "modules/notifications/utils";
import { type FC, Fragment } from "react";
import { useMutation, useQueryClient } from "react-query";
import { toast } from "sonner";
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
		<div className="flex flex-col gap-8">
			{hasWebhookNotifications && !isWebhookConfigured && (
				<Alert
					severity="warning"
					prominent
					actions={
						<Button variant="subtle" size="sm" asChild>
							<a
								target="_blank"
								rel="noreferrer"
								href={docs("/admin/monitoring/notifications#webhook")}
							>
								Read the docs
							</a>
						</Button>
					}
				>
					Webhook notifications are enabled, but not properly configured.
				</Alert>
			)}

			{hasSMTPNotifications && !isSMTPConfigured && (
				<Alert
					severity="warning"
					prominent
					actions={
						<Button variant="subtle" size="sm" asChild>
							<a
								target="_blank"
								rel="noreferrer"
								href={docs("/admin/monitoring/notifications#smtp-email")}
							>
								Read the docs
							</a>
						</Button>
					}
				>
					SMTP notifications are enabled but not properly configured.
				</Alert>
			)}

			{Object.entries(templatesByGroup).map(([group, templates]) => (
				<article
					className="w-full overflow-hidden rounded-lg border border-solid"
					key={group}
				>
					<div className="flex flex-col">
						<header className="border-0 border-b border-solid bg-surface-secondary px-4 py-3">
							<h3 className="text-sm font-medium">{group}</h3>
						</header>

						{templates.map((tpl, i) => {
							const value = castNotificationMethod(tpl.method || defaultMethod);
							const isLastItem = i === templates.length - 1;

							return (
								<Fragment key={tpl.id}>
									<div
										className={`flex items-center justify-between gap-3 px-4 py-3 border-0 border-solid ${
											isLastItem ? "" : "border-b"
										}`}
									>
										<span className="text-sm font-medium">{tpl.name}</span>
										<MethodToggleGroup
											templateId={tpl.id}
											options={availableMethods}
											value={value}
										/>
									</div>
								</Fragment>
							);
						})}
					</div>
				</article>
			))}
		</div>
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
		<ToggleGroup
			type="single"
			value={value}
			variant="outline"
			aria-label="Notification method"
			onValueChange={async (method) => {
				// Keep one method selected and ignore empty deselection.
				if (!method || method === value) {
					return;
				}

				if (!options.includes(method as NotificationMethod)) {
					return;
				}

				try {
					await updateMethodMutation.mutateAsync({
						method: method as NotificationMethod,
					});
					toast.success("Notification method updated.");
				} catch (error) {
					toast.error(
						getErrorMessage(error, "Failed to update notification method."),
						{
							description: getErrorDetail(error),
						},
					);
				}
			}}
		>
			{options.map((method) => {
				const Icon = methodIcons[method];
				const label = methodLabels[method];
				return (
					<Tooltip key={method}>
						<ToggleGroupItem value={method}>
							<TooltipTrigger asChild>
								<span className="inline-flex">
									<Icon aria-label={label} />
								</span>
							</TooltipTrigger>
						</ToggleGroupItem>
						<TooltipContent side="bottom" sideOffset={8}>
							{label}
						</TooltipContent>
					</Tooltip>
				);
			})}
		</ToggleGroup>
	);
};
