import type { FC } from "react";
import { useMutation, useQueryClient } from "react-query";
import { toast } from "sonner";
import { getErrorDetail, getErrorMessage } from "#/api/errors";
import {
	type selectTemplatesByGroup,
	updateNotificationTemplateMethod,
} from "#/api/queries/notifications";
import type { DeploymentValues } from "#/api/typesGenerated";
import { Alert } from "#/components/Alert/Alert";
import { Link } from "#/components/Link/Link";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "#/components/Select/Select";
import {
	castNotificationMethod,
	methodIcons,
	methodLabels,
	type NotificationMethod,
} from "#/modules/notifications/utils";
import { docs } from "#/utils/docs";

type NotificationEventsProps = {
	defaultMethod: NotificationMethod;
	availableMethods: NotificationMethod[];
	templatesByGroup: ReturnType<typeof selectTemplatesByGroup>;
	deploymentConfig: DeploymentValues;
	canEdit?: boolean;
};

export const NotificationEvents: FC<NotificationEventsProps> = ({
	defaultMethod,
	availableMethods,
	templatesByGroup,
	deploymentConfig,
	canEdit = true,
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
						<Link
							href={docs("/admin/monitoring/notifications#webhook")}
							target="_blank"
						>
							View docs
							<span className="sr-only"> (opens in new tab)</span>
						</Link>
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
						<Link
							href={docs("/admin/monitoring/notifications#smtp-email")}
							target="_blank"
						>
							View docs
							<span className="sr-only"> (opens in new tab)</span>
						</Link>
					}
				>
					SMTP notifications are enabled but not properly configured.
				</Alert>
			)}

			{Object.entries(templatesByGroup).map(([group, templates]) => (
				<article
					key={group}
					className="border border-solid rounded-md overflow-hidden w-full"
				>
					<header className="bg-surface-secondary border-0 border-b border-solid px-4 py-3">
						<span className="font-medium text-sm">{group}</span>
					</header>

					{templates.map((tpl) => {
						const value = castNotificationMethod(tpl.method || defaultMethod);

						return (
							<div
								key={tpl.id}
								data-testid="notification-template-row"
								className="flex items-center justify-between gap-3 px-4 py-3 border-0 not-last:border-b border-solid"
							>
								<span className="font-medium text-sm">{tpl.name}</span>
								<MethodSelect
									templateId={tpl.id}
									templateName={tpl.name}
									options={availableMethods}
									value={value}
									canEdit={canEdit}
								/>
							</div>
						);
					})}
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

type MethodSelectProps = {
	templateId: string;
	templateName: string;
	options: NotificationMethod[];
	value: NotificationMethod;
	canEdit: boolean;
};

const MethodSelect: FC<MethodSelectProps> = ({
	value,
	options,
	templateId,
	templateName,
	canEdit,
}) => {
	const queryClient = useQueryClient();
	const updateMethodMutation = useMutation(
		updateNotificationTemplateMethod(templateId, queryClient),
	);

	const SelectedIcon = methodIcons[value];

	return (
		<Select
			value={value}
			disabled={!canEdit}
			onValueChange={async (method) => {
				if (method === value) {
					return;
				}
				try {
					await updateMethodMutation.mutateAsync({
						method,
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
			<SelectTrigger
				aria-label={`Notification method for ${templateName}`}
				className="h-8 w-auto min-w-32 gap-2 [&>span]:flex [&>span]:items-center [&>span]:gap-2 [&>span]:line-clamp-none"
			>
				<SelectValue>
					<span className="flex items-center gap-2 leading-none">
						<SelectedIcon className="size-icon-sm shrink-0" aria-hidden />
						{methodLabels[value]}
					</span>
				</SelectValue>
			</SelectTrigger>
			<SelectContent align="end">
				{options.map((method) => {
					const Icon = methodIcons[method];
					const label = methodLabels[method];
					return (
						<SelectItem key={method} value={method}>
							<span className="flex items-center gap-2 leading-none">
								<Icon className="size-icon-sm shrink-0" aria-hidden />
								{label}
							</span>
						</SelectItem>
					);
				})}
			</SelectContent>
		</Select>
	);
};
