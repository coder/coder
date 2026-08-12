import { type FormikTouched, useFormik } from "formik";
import { type FC, useEffect, useState } from "react";
import type { Template, UpdateTemplateMeta } from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import { Checkbox } from "#/components/Checkbox/Checkbox";
import { DurationField } from "#/components/DurationField/DurationField";
import {
	FormFields,
	FormFooter,
	FormSection,
	HorizontalForm,
} from "#/components/Form/Form";
import { FormField } from "#/components/FormField/FormField";
import { Label } from "#/components/Label/Label";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "#/components/Select/Select";
import { Spinner } from "#/components/Spinner/Spinner";
import {
	StackLabel,
	StackLabelHelperText,
} from "#/components/StackLabel/StackLabel";
import { Switch } from "#/components/Switch/Switch";
import { cn } from "#/utils/cn";
import { getFormHelpers } from "#/utils/formUtils";
import {
	calculateAutostopRequirementDaysValue,
	type TemplateAutostartRequirementDaysValue,
} from "#/utils/schedule";
import {
	AutostopRequirementDaysHelperText,
	AutostopRequirementWeeksHelperText,
	convertAutostopRequirementDaysValue,
} from "./AutostopRequirementHelperText";
import {
	getValidationSchema,
	type TemplateScheduleFormValues,
} from "./formHelpers";
import { ScheduleDialog } from "./ScheduleDialog";
import { TemplateScheduleAutostart } from "./TemplateScheduleAutostart";
import {
	ActivityBumpHelperText,
	AutostopReminderHelperText,
	DefaultTTLHelperText,
	DormancyAutoDeletionTTLHelperText,
	DormancyTTLHelperText,
	FailureTTLHelperText,
} from "./TTLHelperText";
import {
	useWorkspacesToBeDeleted,
	useWorkspacesToGoDormant,
} from "./useWorkspacesToBeDeleted";

const MS_HOUR_CONVERSION = 3600000;
const MS_DAY_CONVERSION = 86400000;
const FAILURE_CLEANUP_DEFAULT = 7 * MS_DAY_CONVERSION;
const INACTIVITY_CLEANUP_DEFAULT = 180 * MS_DAY_CONVERSION;
const DORMANT_AUTODELETION_DEFAULT = 30 * MS_DAY_CONVERSION;

export interface TemplateScheduleForm {
	template: Template;
	onSubmit: (data: UpdateTemplateMeta) => void;
	onCancel: () => void;
	isSubmitting: boolean;
	error?: unknown;
	allowAdvancedScheduling: boolean;
	// Helpful to show field errors on Storybook
	initialTouched?: FormikTouched<UpdateTemplateMeta>;
}

export const TemplateScheduleForm: FC<TemplateScheduleForm> = ({
	template,
	onSubmit,
	onCancel,
	error,
	allowAdvancedScheduling,
	isSubmitting,
	initialTouched,
}) => {
	const validationSchema = getValidationSchema();
	const form = useFormik<TemplateScheduleFormValues>({
		initialValues: {
			// on display, convert from ms => hours
			default_ttl_ms: template.default_ttl_ms / MS_HOUR_CONVERSION,
			activity_bump_ms: template.activity_bump_ms / MS_HOUR_CONVERSION,
			time_til_autostop_notify_ms:
				template.time_til_autostop_notify_ms / MS_HOUR_CONVERSION,
			failure_ttl_ms: template.failure_ttl_ms,
			time_til_dormant_ms: template.time_til_dormant_ms,
			time_til_dormant_autodelete_ms: template.time_til_dormant_autodelete_ms,
			autostop_requirement_days_of_week: allowAdvancedScheduling
				? convertAutostopRequirementDaysValue(
						template.autostop_requirement.days_of_week,
					)
				: "off",
			autostop_requirement_weeks: allowAdvancedScheduling
				? template.autostop_requirement.weeks > 0
					? template.autostop_requirement.weeks
					: 1
				: 1,
			autostart_requirement_days_of_week: template.autostart_requirement
				.days_of_week as TemplateAutostartRequirementDaysValue[],

			allow_user_autostart: template.allow_user_autostart,
			allow_user_autostop: template.allow_user_autostop,
			failure_cleanup_enabled:
				allowAdvancedScheduling && Boolean(template.failure_ttl_ms),
			inactivity_cleanup_enabled:
				allowAdvancedScheduling && Boolean(template.time_til_dormant_ms),
			dormant_autodeletion_cleanup_enabled:
				allowAdvancedScheduling &&
				Boolean(template.time_til_dormant_autodelete_ms),
			update_workspace_last_used_at: false,
			update_workspace_dormant_at: false,
			require_active_version: false,
			disable_everyone_group_access: false,
		},
		validationSchema,
		onSubmit: () => {
			const dormancyChanged =
				form.initialValues.time_til_dormant_ms !==
				form.values.time_til_dormant_ms;
			const deletionChanged =
				form.initialValues.time_til_dormant_autodelete_ms !==
				form.values.time_til_dormant_autodelete_ms;

			const dormancyScheduleChanged =
				form.values.inactivity_cleanup_enabled &&
				dormancyChanged &&
				workspacesToDormancyInWeek &&
				workspacesToDormancyInWeek.length > 0;

			const deletionScheduleChanged =
				form.values.inactivity_cleanup_enabled &&
				deletionChanged &&
				workspacesToBeDeletedInWeek &&
				workspacesToBeDeletedInWeek.length > 0;

			if (dormancyScheduleChanged || deletionScheduleChanged) {
				setIsScheduleDialogOpen(true);
			} else {
				submitValues();
			}
		},
		initialTouched,
		enableReinitialize: true,
	});

	const getFieldHelpers = getFormHelpers<TemplateScheduleFormValues>(
		form,
		error,
	);
	const autostopDaysField = getFieldHelpers(
		"autostop_requirement_days_of_week",
		{
			helperText: (
				<AutostopRequirementDaysHelperText
					days={form.values.autostop_requirement_days_of_week}
				/>
			),
		},
	);
	const autostopDaysHelperId = `${autostopDaysField.id}-helper`;
	const timeTilDormantField = getFieldHelpers("time_til_dormant_ms", {
		helperText: <DormancyTTLHelperText ttl={form.values.time_til_dormant_ms} />,
	});
	const timeTilDormantAutodeleteField = getFieldHelpers(
		"time_til_dormant_autodelete_ms",
		{
			helperText: (
				<DormancyAutoDeletionTTLHelperText
					ttl={form.values.time_til_dormant_autodelete_ms}
				/>
			),
		},
	);
	const failureTtlField = getFieldHelpers("failure_ttl_ms", {
		helperText: <FailureTTLHelperText ttl={form.values.failure_ttl_ms} />,
	});

	const now = new Date();
	const weekFromNow = new Date(now);
	weekFromNow.setDate(now.getDate() + 7);

	const workspacesToDormancyNow = useWorkspacesToGoDormant(
		template,
		form.values,
		now,
	);

	const workspacesToDormancyInWeek = useWorkspacesToGoDormant(
		template,
		form.values,
		weekFromNow,
	);

	const workspacesToBeDeletedNow = useWorkspacesToBeDeleted(
		template,
		form.values,
		now,
	);

	const workspacesToBeDeletedInWeek = useWorkspacesToBeDeleted(
		template,
		form.values,
		weekFromNow,
	);

	const showScheduleDialog =
		workspacesToDormancyNow &&
		workspacesToBeDeletedNow &&
		workspacesToDormancyInWeek &&
		workspacesToBeDeletedInWeek &&
		(workspacesToDormancyInWeek.length > 0 ||
			workspacesToBeDeletedInWeek.length > 0);

	const [isScheduleDialogOpen, setIsScheduleDialogOpen] =
		useState<boolean>(false);

	const submitValues = () => {
		const autostop_requirement_weeks = ["saturday", "sunday"].includes(
			form.values.autostop_requirement_days_of_week,
		)
			? form.values.autostop_requirement_weeks
			: 1;

		// on submit, convert from hours => ms
		onSubmit({
			default_ttl_ms: form.values.default_ttl_ms
				? form.values.default_ttl_ms * MS_HOUR_CONVERSION
				: undefined,
			// Activity bump has no effect without a scheduled stop time, so
			// discard any stale value when there is no default TTL AND users
			// cannot customize autostop on their workspaces.
			activity_bump_ms:
				(form.values.default_ttl_ms || form.values.allow_user_autostop) &&
				form.values.activity_bump_ms
					? form.values.activity_bump_ms * MS_HOUR_CONVERSION
					: undefined,
			// 0 disables the reminder, so always send an explicit value.
			time_til_autostop_notify_ms: form.values.time_til_autostop_notify_ms
				? form.values.time_til_autostop_notify_ms * MS_HOUR_CONVERSION
				: 0,
			failure_ttl_ms: form.values.failure_ttl_ms,
			time_til_dormant_ms: form.values.time_til_dormant_ms,
			time_til_dormant_autodelete_ms:
				form.values.time_til_dormant_autodelete_ms,
			autostop_requirement: {
				days_of_week: calculateAutostopRequirementDaysValue(
					form.values.autostop_requirement_days_of_week,
				),
				weeks: autostop_requirement_weeks,
			},
			autostart_requirement: {
				days_of_week: form.values.autostart_requirement_days_of_week,
			},
			allow_user_autostart: form.values.allow_user_autostart,
			allow_user_autostop: form.values.allow_user_autostop,
			update_workspace_last_used_at: form.values.update_workspace_last_used_at,
			update_workspace_dormant_at: form.values.update_workspace_dormant_at,
			disable_everyone_group_access: false,
		});
	};

	// Set autostop_requirement weeks to 1 when days_of_week is set to "off" or
	// "daily". Technically you can set weeks to a different value in the backend
	// and it will work, but this is a UX decision so users don't set days=daily
	// and weeks=2 and get confused when workspaces only restart daily during
	// every second week.
	//
	// We want to set the value to 1 when the user selects "off" or "daily"
	// because the input gets disabled so they can't change it to 1 themselves.
	const { values: currentValues, setValues } = form;
	useEffect(() => {
		if (
			!["saturday", "sunday"].includes(
				currentValues.autostop_requirement_days_of_week,
			) &&
			currentValues.autostop_requirement_weeks !== 1
		) {
			// This is async but we don't really need to await the value.
			setValues({
				...currentValues,
				autostop_requirement_weeks: 1,
			});
		}
	}, [currentValues, setValues]);

	const handleToggleFailureCleanup = async (checked: boolean) => {
		await form.setValues({
			...form.values,
			failure_cleanup_enabled: checked,
			failure_ttl_ms: checked ? FAILURE_CLEANUP_DEFAULT : 0,
		});
	};

	const handleToggleInactivityCleanup = async (checked: boolean) => {
		await form.setValues({
			...form.values,
			inactivity_cleanup_enabled: checked,
			time_til_dormant_ms: checked ? INACTIVITY_CLEANUP_DEFAULT : 0,
		});
	};

	const handleToggleDormantAutoDeletion = async (checked: boolean) => {
		await form.setValues({
			...form.values,
			dormant_autodeletion_cleanup_enabled: checked,
			time_til_dormant_autodelete_ms: checked
				? DORMANT_AUTODELETION_DEFAULT
				: 0,
		});
	};

	return (
		<HorizontalForm
			onSubmit={form.handleSubmit}
			aria-label="Template settings form"
		>
			<FormSection
				title="Autostop"
				description="Define when workspaces created from this template are stopped."
			>
				<FormFields>
					<FormField
						field={getFieldHelpers("default_ttl_ms", {
							helperText: (
								<DefaultTTLHelperText ttl={form.values.default_ttl_ms} />
							),
						})}
						label="Default autostop (hours)"
						type="number"
						disabled={isSubmitting}
						min={0}
						step={1}
						className="w-full"
					/>

					<FormField
						field={getFieldHelpers("activity_bump_ms", {
							helperText: (
								<ActivityBumpHelperText
									bump={form.values.activity_bump_ms}
									defaultTTL={form.values.default_ttl_ms}
									allowUserAutostop={form.values.allow_user_autostop}
								/>
							),
						})}
						label="Activity bump (hours)"
						type="number"
						disabled={
							isSubmitting ||
							(!form.values.default_ttl_ms && !form.values.allow_user_autostop)
						}
						min={0}
						step={1}
						className="w-full"
					/>

					<FormField
						field={getFieldHelpers("time_til_autostop_notify_ms", {
							helperText: (
								<AutostopReminderHelperText
									lead={form.values.time_til_autostop_notify_ms}
									defaultTTL={form.values.default_ttl_ms}
									autostopRequirementDaysOfWeek={
										form.values.autostop_requirement_days_of_week
									}
									allowUserAutostop={form.values.allow_user_autostop}
								/>
							),
						})}
						label="Autostop reminder (hours)"
						type="number"
						disabled={isSubmitting}
						min={0}
						step={1}
						className="w-full"
					/>

					<div className="grid grid-cols-2 gap-4 w-full items-start">
						<div className="flex flex-col gap-2 min-w-0">
							<Label htmlFor={autostopDaysField.id}>
								Days with required stop
							</Label>
							<Select
								value={form.values.autostop_requirement_days_of_week}
								onValueChange={(value) => {
									form.setFieldValue(
										"autostop_requirement_days_of_week",
										value,
									);
								}}
								disabled={isSubmitting}
							>
								<SelectTrigger
									id={autostopDaysField.id}
									className={cn(
										"w-full",
										autostopDaysField.error && "border-border-destructive",
									)}
									aria-invalid={autostopDaysField.error}
									aria-describedby={
										autostopDaysField.helperText
											? autostopDaysHelperId
											: undefined
									}
								>
									<SelectValue />
								</SelectTrigger>
								<SelectContent>
									<SelectItem value="off">Off</SelectItem>
									<SelectItem value="daily">Daily</SelectItem>
									<SelectItem value="saturday">Saturday</SelectItem>
									<SelectItem value="sunday">Sunday</SelectItem>
								</SelectContent>
							</Select>
							{autostopDaysField.helperText && (
								<span
									id={autostopDaysHelperId}
									className={cn(
										"text-xs",
										autostopDaysField.error
											? "text-content-destructive"
											: "text-content-secondary",
									)}
								>
									{autostopDaysField.helperText}
								</span>
							)}
						</div>

						<div className="min-w-0">
							<FormField
								field={getFieldHelpers("autostop_requirement_weeks", {
									helperText: (
										<AutostopRequirementWeeksHelperText
											days={form.values.autostop_requirement_days_of_week}
											weeks={form.values.autostop_requirement_weeks}
										/>
									),
								})}
								label="Weeks between required stops"
								type="number"
								disabled={
									isSubmitting ||
									!["saturday", "sunday"].includes(
										form.values.autostop_requirement_days_of_week || "",
									)
								}
								min={1}
								max={16}
								step={1}
								className="w-full"
							/>
						</div>
					</div>

					<div className="flex items-start">
						<Checkbox
							id="allow-user-autostop"
							disabled={isSubmitting || !allowAdvancedScheduling}
							onCheckedChange={async (checked) => {
								await form.setFieldValue(
									"allow_user_autostop",
									checked === true,
								);
							}}
							name="allow_user_autostop"
							checked={form.values.allow_user_autostop}
						/>
						<Label htmlFor="allow-user-autostop">
							<StackLabel>
								Allow users to customize autostop duration for workspaces.
								<StackLabelHelperText>
									By default, workspaces will inherit the Autostop timer from
									this template. Enabling this option allows users to set custom
									Autostop timers on their workspaces or turn off the timer.
								</StackLabelHelperText>
							</StackLabel>
						</Label>
					</div>
				</FormFields>
			</FormSection>

			<FormSection
				title="Autostart"
				description="Allow users to set custom autostart and autostop scheduling options for workspaces created from this template."
			>
				<div className="flex flex-col gap-4">
					<div className="flex items-start">
						<Checkbox
							id="allow_user_autostart"
							disabled={isSubmitting || !allowAdvancedScheduling}
							onCheckedChange={async (checked) => {
								await form.setFieldValue(
									"allow_user_autostart",
									checked === true,
								);
							}}
							name="allow_user_autostart"
							checked={form.values.allow_user_autostart}
						/>
						<Label htmlFor="allow_user_autostart">
							<StackLabel>
								Allow users to automatically start workspaces on a schedule.
							</StackLabel>
						</Label>
					</div>

					{allowAdvancedScheduling && (
						<TemplateScheduleAutostart
							enabled={Boolean(form.values.allow_user_autostart)}
							value={form.values.autostart_requirement_days_of_week}
							isSubmitting={isSubmitting}
							onChange={async (
								newDaysOfWeek: TemplateAutostartRequirementDaysValue[],
							) => {
								await form.setFieldValue(
									"autostart_requirement_days_of_week",
									newDaysOfWeek,
								);
							}}
						/>
					)}
				</div>
			</FormSection>

			{allowAdvancedScheduling && (
				<FormSection
					title="Dormancy"
					description="When enabled, Coder will mark workspaces as dormant after a period of time with no connections. Dormant workspaces can be auto-deleted (see below) or manually reviewed by the workspace owner or admins."
				>
					<FormFields>
						<div className="flex flex-col gap-8">
							<div className="flex items-start">
								<Switch
									id="dormancyThreshold"
									name="dormancyThreshold"
									checked={form.values.inactivity_cleanup_enabled}
									onCheckedChange={handleToggleInactivityCleanup}
								/>
								<Label htmlFor="dormancyThreshold">
									<StackLabel>Enable Dormancy Threshold</StackLabel>
								</Label>
							</div>

							<DurationField
								label="Time until dormant"
								valueMs={form.values.time_til_dormant_ms ?? 0}
								onChange={(v) => form.setFieldValue("time_til_dormant_ms", v)}
								disabled={
									isSubmitting || !form.values.inactivity_cleanup_enabled
								}
								id={timeTilDormantField.id}
								name={timeTilDormantField.name}
								onBlur={timeTilDormantField.onBlur}
								error={timeTilDormantField.error}
								helperText={timeTilDormantField.helperText}
							/>
						</div>

						<div className="flex flex-col gap-8">
							<div className="flex items-start">
								<Switch
									id="dormancyAutoDeletion"
									name="dormancyAutoDeletion"
									checked={form.values.dormant_autodeletion_cleanup_enabled}
									onCheckedChange={handleToggleDormantAutoDeletion}
								/>
								<Label htmlFor="dormancyAutoDeletion">
									<StackLabel>
										Enable Dormancy Auto-Deletion
										<StackLabelHelperText>
											When enabled, Coder will permanently delete dormant
											workspaces after a period of time.{" "}
											<strong>
												Once a workspace is deleted it cannot be recovered.
											</strong>
										</StackLabelHelperText>
									</StackLabel>
								</Label>
							</div>
							<DurationField
								label="Time until deletion"
								valueMs={form.values.time_til_dormant_autodelete_ms ?? 0}
								onChange={(v) =>
									form.setFieldValue("time_til_dormant_autodelete_ms", v)
								}
								disabled={
									isSubmitting ||
									!form.values.dormant_autodeletion_cleanup_enabled
								}
								id={timeTilDormantAutodeleteField.id}
								name={timeTilDormantAutodeleteField.name}
								onBlur={timeTilDormantAutodeleteField.onBlur}
								error={timeTilDormantAutodeleteField.error}
								helperText={timeTilDormantAutodeleteField.helperText}
							/>
						</div>

						<div className="flex flex-col gap-8">
							<div className="flex items-start">
								<Switch
									id="failureCleanupEnabled"
									name="failureCleanupEnabled"
									checked={form.values.failure_cleanup_enabled}
									onCheckedChange={handleToggleFailureCleanup}
								/>
								<Label htmlFor="failureCleanupEnabled">
									<StackLabel>
										Enable Failure Cleanup
										<StackLabelHelperText>
											When enabled, Coder will attempt to stop workspaces that
											are in a failed state after a period of time.
										</StackLabelHelperText>
									</StackLabel>
								</Label>
							</div>
							<DurationField
								label="Time until cleanup"
								valueMs={form.values.failure_ttl_ms ?? 0}
								onChange={(v) => form.setFieldValue("failure_ttl_ms", v)}
								disabled={isSubmitting || !form.values.failure_cleanup_enabled}
								id={failureTtlField.id}
								name={failureTtlField.name}
								onBlur={failureTtlField.onBlur}
								error={failureTtlField.error}
								helperText={failureTtlField.helperText}
							/>
						</div>
					</FormFields>
				</FormSection>
			)}
			{showScheduleDialog && (
				<ScheduleDialog
					onConfirm={() => {
						submitValues();
						setIsScheduleDialogOpen(false);
						// These fields are request-scoped so they should be reset
						// after every submission.
						form
							.setFieldValue("update_workspace_dormant_at", false)
							.catch((error) => {
								throw error;
							});
						form
							.setFieldValue("update_workspace_last_used_at", false)
							.catch((error) => {
								throw error;
							});
					}}
					inactiveWorkspacesToGoDormant={workspacesToDormancyNow.length}
					inactiveWorkspacesToGoDormantInWeek={
						workspacesToDormancyInWeek.length - workspacesToDormancyNow.length
					}
					dormantWorkspacesToBeDeleted={workspacesToBeDeletedNow.length}
					dormantWorkspacesToBeDeletedInWeek={
						workspacesToBeDeletedInWeek.length - workspacesToBeDeletedNow.length
					}
					open={isScheduleDialogOpen}
					onClose={() => {
						setIsScheduleDialogOpen(false);
					}}
					title="Workspace Scheduling"
					updateDormantWorkspaces={(update: boolean) =>
						form.setFieldValue("update_workspace_dormant_at", update)
					}
					updateInactiveWorkspaces={(update: boolean) =>
						form.setFieldValue("update_workspace_last_used_at", update)
					}
					dormantWorkspacesChecked={
						form.values.update_workspace_dormant_at ?? false
					}
					inactiveWorkspacesChecked={
						form.values.update_workspace_last_used_at ?? false
					}
					dormantValueChanged={
						form.initialValues.time_til_dormant_ms !==
						form.values.time_til_dormant_ms
					}
					deletionValueChanged={
						form.initialValues.time_til_dormant_autodelete_ms !==
						form.values.time_til_dormant_autodelete_ms
					}
				/>
			)}

			<FormFooter>
				<Button onClick={onCancel} variant="outline">
					Cancel
				</Button>

				<Button
					type="submit"
					disabled={isSubmitting || !form.isValid || !form.dirty}
				>
					<Spinner loading={isSubmitting} />
					Save
				</Button>
			</FormFooter>
		</HorizontalForm>
	);
};
