import type { Template, UpdateTemplateMeta } from "api/typesGenerated";
import { Button } from "components/Button/Button";
import { Checkbox } from "components/Checkbox/Checkbox";
import { DurationField } from "components/DurationField/DurationField";
import {
	FormFields,
	FormFooter,
	FormSection,
	HorizontalForm,
} from "components/Form/Form";
import { Input } from "components/Input/Input";
import { Label } from "components/Label/Label";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "components/Select/Select";
import { Spinner } from "components/Spinner/Spinner";
import { Switch } from "components/Switch/Switch";
import { type FormikTouched, useFormik } from "formik";
import { type FC, useEffect, useState } from "react";
import { cn } from "utils/cn";
import { getFormHelpers } from "utils/formUtils";
import {
	calculateAutostopRequirementDaysValue,
	type TemplateAutostartRequirementDaysValue,
} from "utils/schedule";
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
/**
 * The default form field space is 4 but since this form is quite heavy I think
 * increase the space can make it feels lighter.
 */
const FORM_FIELDS_SPACING = 8;

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

	const defaultTtlField = getFieldHelpers("default_ttl_ms", {
		helperText: <DefaultTTLHelperText ttl={form.values.default_ttl_ms} />,
	});
	const activityBumpField = getFieldHelpers("activity_bump_ms", {
		helperText: <ActivityBumpHelperText bump={form.values.activity_bump_ms} />,
	});
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
	const autostopWeeksField = getFieldHelpers("autostop_requirement_weeks", {
		helperText: (
			<AutostopRequirementWeeksHelperText
				days={form.values.autostop_requirement_days_of_week}
				weeks={form.values.autostop_requirement_weeks}
			/>
		),
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
			activity_bump_ms: form.values.activity_bump_ms
				? form.values.activity_bump_ms * MS_HOUR_CONVERSION
				: undefined,
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
			void setValues({
				...currentValues,
				autostop_requirement_weeks: 1,
			});
		}
	}, [currentValues, setValues]);

	const handleToggleFailureCleanup = async () => {
		if (!form.values.failure_cleanup_enabled) {
			await form.setValues({
				...form.values,
				failure_cleanup_enabled: true,
				failure_ttl_ms: FAILURE_CLEANUP_DEFAULT,
			});
		} else {
			await form.setValues({
				...form.values,
				failure_cleanup_enabled: false,
				failure_ttl_ms: 0,
			});
		}
	};

	const handleToggleInactivityCleanup = async () => {
		if (!form.values.inactivity_cleanup_enabled) {
			await form.setValues({
				...form.values,
				inactivity_cleanup_enabled: true,
				time_til_dormant_ms: INACTIVITY_CLEANUP_DEFAULT,
			});
		} else {
			await form.setValues({
				...form.values,
				inactivity_cleanup_enabled: false,
				time_til_dormant_ms: 0,
			});
		}
	};

	const handleToggleDormantAutoDeletion = async () => {
		if (!form.values.dormant_autodeletion_cleanup_enabled) {
			await form.setValues({
				...form.values,
				dormant_autodeletion_cleanup_enabled: true,
				time_til_dormant_autodelete_ms: DORMANT_AUTODELETION_DEFAULT,
			});
		} else {
			await form.setValues({
				...form.values,
				dormant_autodeletion_cleanup_enabled: false,
				time_til_dormant_autodelete_ms: 0,
			});
		}
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
				<FormFields spacing={FORM_FIELDS_SPACING}>
					<div className="flex flex-col items-start gap-2">
						<Label htmlFor={defaultTtlField.id}>Default autostop (hours)</Label>
						<Input
							id={defaultTtlField.id}
							name={defaultTtlField.name}
							value={defaultTtlField.value}
							onChange={defaultTtlField.onChange}
							onBlur={defaultTtlField.onBlur}
							disabled={isSubmitting}
							type="number"
							min={0}
							step={1}
							aria-invalid={defaultTtlField.error}
						/>
						{defaultTtlField.helperText && (
							<span
								className={cn(
									"text-xs",
									defaultTtlField.error
										? "text-content-destructive"
										: "text-content-secondary",
								)}
							>
								{defaultTtlField.helperText}
							</span>
						)}
					</div>

					<div className="flex flex-col items-start gap-2">
						<Label htmlFor={activityBumpField.id}>Activity bump (hours)</Label>
						<Input
							id={activityBumpField.id}
							name={activityBumpField.name}
							value={activityBumpField.value}
							onChange={activityBumpField.onChange}
							onBlur={activityBumpField.onBlur}
							disabled={isSubmitting}
							type="number"
							min={0}
							step={1}
							aria-invalid={activityBumpField.error}
						/>
						{activityBumpField.helperText && (
							<span
								className={cn(
									"text-xs",
									activityBumpField.error
										? "text-content-destructive"
										: "text-content-secondary",
								)}
							>
								{activityBumpField.helperText}
							</span>
						)}
					</div>

					<div className="flex gap-4 w-full">
						<div className="flex flex-col items-start gap-2 flex-1">
							<Label htmlFor={autostopDaysField.id}>
								Days with required stop
							</Label>
							<Select
								value={form.values.autostop_requirement_days_of_week}
								onValueChange={(value) =>
									form.setFieldValue("autostop_requirement_days_of_week", value)
								}
								disabled={isSubmitting}
							>
								<SelectTrigger id={autostopDaysField.id}>
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

						<div className="flex flex-col items-start gap-2 flex-1">
							<Label htmlFor={autostopWeeksField.id}>
								Weeks between required stops
							</Label>
							<Input
								id={autostopWeeksField.id}
								name={autostopWeeksField.name}
								value={autostopWeeksField.value}
								onChange={autostopWeeksField.onChange}
								onBlur={autostopWeeksField.onBlur}
								disabled={
									isSubmitting ||
									!["saturday", "sunday"].includes(
										form.values.autostop_requirement_days_of_week || "",
									)
								}
								type="number"
								min={1}
								max={16}
								step={1}
								aria-invalid={autostopWeeksField.error}
							/>
							{autostopWeeksField.helperText && (
								<span
									className={cn(
										"text-xs",
										autostopWeeksField.error
											? "text-content-destructive"
											: "text-content-secondary",
									)}
								>
									{autostopWeeksField.helperText}
								</span>
							)}
						</div>
					</div>

					<div className="flex items-start gap-3">
						<Checkbox
							id="allow-user-autostop"
							checked={form.values.allow_user_autostop}
							onCheckedChange={(checked) =>
								form.setFieldValue("allow_user_autostop", checked === true)
							}
							disabled={isSubmitting || !allowAdvancedScheduling}
							className="mt-0.5"
						/>
						<label
							htmlFor="allow-user-autostop"
							className="flex flex-col gap-1 cursor-pointer"
						>
							<span className="text-sm font-medium">
								Allow users to customize autostop duration for workspaces.
							</span>
							<span className="text-xs text-content-secondary">
								By default, workspaces will inherit the Autostop timer from this
								template. Enabling this option allows users to set custom
								Autostop timers on their workspaces or turn off the timer.
							</span>
						</label>
					</div>
				</FormFields>
			</FormSection>

			<FormSection
				title="Autostart"
				description="Allow users to set custom autostart and autostop scheduling options for workspaces created from this template."
			>
				<div className="flex flex-col gap-4">
					<div className="flex items-start gap-3">
						<Checkbox
							id="allow_user_autostart"
							checked={form.values.allow_user_autostart}
							onCheckedChange={(checked) =>
								form.setFieldValue("allow_user_autostart", checked === true)
							}
							disabled={isSubmitting || !allowAdvancedScheduling}
							className="mt-0.5"
						/>
						<label
							htmlFor="allow_user_autostart"
							className="text-sm font-medium cursor-pointer"
						>
							Allow users to automatically start workspaces on a schedule.
						</label>
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
					<FormFields spacing={FORM_FIELDS_SPACING}>
						<div className="flex flex-col gap-8">
							<div className="flex items-center gap-3">
								<Switch
									id="dormancyThreshold"
									checked={form.values.inactivity_cleanup_enabled}
									onCheckedChange={handleToggleInactivityCleanup}
								/>
								<label
									htmlFor="dormancyThreshold"
									className="text-sm font-medium cursor-pointer"
								>
									Enable Dormancy Threshold
								</label>
							</div>

							<DurationField
								{...getFieldHelpers("time_til_dormant_ms", {
									helperText: (
										<DormancyTTLHelperText
											ttl={form.values.time_til_dormant_ms}
										/>
									),
								})}
								label="Time until dormant"
								valueMs={form.values.time_til_dormant_ms ?? 0}
								onChange={(v) => form.setFieldValue("time_til_dormant_ms", v)}
								disabled={
									isSubmitting || !form.values.inactivity_cleanup_enabled
								}
							/>
						</div>

						<div className="flex flex-col gap-8">
							<div className="flex items-center gap-3">
								<Switch
									id="dormancyAutoDeletion"
									checked={form.values.dormant_autodeletion_cleanup_enabled}
									onCheckedChange={handleToggleDormantAutoDeletion}
								/>
								<label
									htmlFor="dormancyAutoDeletion"
									className="flex flex-col gap-1 cursor-pointer"
								>
									<span className="text-sm font-medium">
										Enable Dormancy Auto-Deletion
									</span>
									<span className="text-xs text-content-secondary">
										When enabled, Coder will permanently delete dormant
										workspaces after a period of time.{" "}
										<strong className="text-content-primary">
											Once a workspace is deleted it cannot be recovered.
										</strong>
									</span>
								</label>
							</div>
							<DurationField
								{...getFieldHelpers("time_til_dormant_autodelete_ms", {
									helperText: (
										<DormancyAutoDeletionTTLHelperText
											ttl={form.values.time_til_dormant_autodelete_ms}
										/>
									),
								})}
								label="Time until deletion"
								valueMs={form.values.time_til_dormant_autodelete_ms ?? 0}
								onChange={(v) =>
									form.setFieldValue("time_til_dormant_autodelete_ms", v)
								}
								disabled={
									isSubmitting ||
									!form.values.dormant_autodeletion_cleanup_enabled
								}
							/>
						</div>

						<div className="flex flex-col gap-8">
							<div className="flex items-center gap-3">
								<Switch
									id="failureCleanupEnabled"
									checked={form.values.failure_cleanup_enabled}
									onCheckedChange={handleToggleFailureCleanup}
								/>
								<label
									htmlFor="failureCleanupEnabled"
									className="flex flex-col gap-1 cursor-pointer"
								>
									<span className="text-sm font-medium">
										Enable Failure Cleanup
									</span>
									<span className="text-xs text-content-secondary">
										When enabled, Coder will attempt to stop workspaces that are
										in a failed state after a period of time.
									</span>
								</label>
							</div>
							<DurationField
								{...getFieldHelpers("failure_ttl_ms", {
									helperText: (
										<FailureTTLHelperText ttl={form.values.failure_ttl_ms} />
									),
								})}
								label="Time until cleanup"
								valueMs={form.values.failure_ttl_ms ?? 0}
								onChange={(v) => form.setFieldValue("failure_ttl_ms", v)}
								disabled={isSubmitting || !form.values.failure_cleanup_enabled}
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
