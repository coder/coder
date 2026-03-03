import type { Template } from "api/typesGenerated";
import { Button } from "components/Button/Button";
import { Checkbox } from "components/Checkbox/Checkbox";
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
import dayjs from "dayjs";
import timezone from "dayjs/plugin/timezone";
import { type FormikTouched, useFormik } from "formik";
import {
	defaultSchedule,
	emptySchedule,
} from "pages/WorkspaceSettingsPage/WorkspaceSchedulePage/schedule";
import type { FC } from "react";
import { getFormHelpers } from "utils/formUtils";
import { humanDuration } from "utils/time";
import { timeZones } from "utils/timeZones";
import * as Yup from "yup";

// Need dayjs.tz functions for timezone validation
dayjs.extend(timezone);

export const Language = {
	errorNoDayOfWeek:
		"Must set at least one day of week if autostart is enabled.",
	errorNoTime: "Start time is required when autostart is enabled.",
	errorTime: "Time must be in HH:mm format.",
	errorTimezone: "Invalid timezone.",
	errorNoStop:
		"Time until shutdown must be greater than zero when autostop is enabled.",
	errorTtlMax:
		"Please enter a limit that is less than or equal to 720 hours (30 days).",
	daysOfWeekLabel: "Days of Week",
	daySundayLabel: "Sun",
	dayMondayLabel: "Mon",
	dayTuesdayLabel: "Tue",
	dayWednesdayLabel: "Wed",
	dayThursdayLabel: "Thu",
	dayFridayLabel: "Fri",
	daySaturdayLabel: "Sat",
	startTimeLabel: "Start time",
	timezoneLabel: "Timezone",
	ttlLabel: "Time until shutdown (hours)",
	formTitle: "Workspace schedule",
	startSection: "Start",
	startSwitch: "Enable Autostart",
	stopSection: "Stop",
	stopSwitch: "Enable Autostop",
};

export interface WorkspaceScheduleFormProps {
	template: Template;
	error?: unknown;
	initialValues: WorkspaceScheduleFormValues;
	isLoading: boolean;
	onCancel: () => void;
	onSubmit: (values: WorkspaceScheduleFormValues) => void;
	// for storybook
	initialTouched?: FormikTouched<WorkspaceScheduleFormValues>;
	defaultTTL: number;
}

export interface WorkspaceScheduleFormValues {
	autostartEnabled: boolean;
	sunday: boolean;
	monday: boolean;
	tuesday: boolean;
	wednesday: boolean;
	thursday: boolean;
	friday: boolean;
	saturday: boolean;
	startTime: string;
	timezone: string;
	autostopEnabled: boolean;
	ttl: number;
}

export const validationSchema = Yup.object({
	sunday: Yup.boolean(),
	monday: Yup.boolean().test(
		"at-least-one-day",
		Language.errorNoDayOfWeek,
		function (value) {
			const parent = this.parent as WorkspaceScheduleFormValues;

			if (!parent.autostartEnabled) {
				return true;
			}

			// Ensure at least one day is enabled
			return [
				parent.sunday,
				value,
				parent.tuesday,
				parent.wednesday,
				parent.thursday,
				parent.friday,
				parent.saturday,
			].some((day) => day);
		},
	),
	tuesday: Yup.boolean(),
	wednesday: Yup.boolean(),
	thursday: Yup.boolean(),
	friday: Yup.boolean(),
	saturday: Yup.boolean(),

	startTime: Yup.string()
		.ensure()
		.test("required-if-autostart", Language.errorNoTime, function (value) {
			const parent = this.parent as WorkspaceScheduleFormValues;
			if (parent.autostartEnabled) {
				return value !== "";
			}
			return true;
		})
		.test("is-time-string", Language.errorTime, (value) => {
			if (value === "") {
				return true;
			}
			if (!/^[0-9][0-9]:[0-9][0-9]$/.test(value)) {
				return false;
			}
			const parts = value.split(":");
			const HH = Number(parts[0]);
			const mm = Number(parts[1]);
			return HH >= 0 && HH <= 23 && mm >= 0 && mm <= 59;
		}),
	timezone: Yup.string()
		.ensure()
		.test("is-timezone", Language.errorTimezone, function (value) {
			const parent = this.parent as WorkspaceScheduleFormValues;

			if (!parent.startTime) {
				return true;
			}
			// Unfortunately, there's not a good API on dayjs at this time for
			// evaluating a timezone. Attempt to parse today in the supplied timezone
			// and return as valid if the function doesn't throw.
			// Need to use dayjs.tz directly here as our utility functions don't expose validation
			try {
				dayjs.tz(dayjs(), value);
				return true;
			} catch {
				return false;
			}
		}),
	ttl: Yup.number()
		.min(0)
		.max(24 * 30 /* 30 days */, Language.errorTtlMax)
		.test("positive-if-autostop", Language.errorNoStop, function (value) {
			const parent = this.parent as WorkspaceScheduleFormValues;
			if (parent.autostopEnabled) {
				return Boolean(value);
			}
			return true;
		}),
});

export const WorkspaceScheduleForm: FC<WorkspaceScheduleFormProps> = ({
	error,
	initialValues,
	isLoading,
	onCancel,
	onSubmit,
	initialTouched,
	defaultTTL,
	template,
}) => {
	const form = useFormik<WorkspaceScheduleFormValues>({
		initialValues,
		onSubmit,
		validationSchema,
		initialTouched,
		enableReinitialize: true,
	});
	const formHelpers = getFormHelpers<WorkspaceScheduleFormValues>(form, error);

	const checkboxes: Array<{ value: boolean; name: string; label: string }> = [
		{
			value: form.values.monday,
			name: "monday",
			label: Language.dayMondayLabel,
		},
		{
			value: form.values.tuesday,
			name: "tuesday",
			label: Language.dayTuesdayLabel,
		},
		{
			value: form.values.wednesday,
			name: "wednesday",
			label: Language.dayWednesdayLabel,
		},
		{
			value: form.values.thursday,
			name: "thursday",
			label: Language.dayThursdayLabel,
		},
		{
			value: form.values.friday,
			name: "friday",
			label: Language.dayFridayLabel,
		},
		{
			value: form.values.saturday,
			name: "saturday",
			label: Language.daySaturdayLabel,
		},
		{
			value: form.values.sunday,
			name: "sunday",
			label: Language.daySundayLabel,
		},
	];

	const startTimeField = formHelpers("startTime");
	const timezoneField = formHelpers("timezone");
	const ttlField = formHelpers("ttl", {
		helperText: ttlShutdownAt(form.values.ttl),
		backendFieldName: "ttl_ms",
	});

	const autostartDisabled =
		isLoading ||
		!template.allow_user_autostart ||
		!form.values.autostartEnabled;

	const autostopDisabled =
		isLoading || !template.allow_user_autostop || !form.values.autostopEnabled;

	return (
		<HorizontalForm onSubmit={form.handleSubmit}>
			<FormSection
				title="Autostart"
				description="Select the time and days of week on which you want the workspace starting automatically."
			>
				<FormFields>
					<div className="flex items-center gap-3">
						<Switch
							id="autostartEnabled"
							disabled={!template.allow_user_autostart}
							checked={form.values.autostartEnabled}
							onCheckedChange={(checked) => {
								void form.setValues({
									...form.values,
									autostartEnabled: checked,
									...(checked ? defaultSchedule() : emptySchedule),
								});
							}}
						/>
						<div className="flex flex-col">
							<Label
								htmlFor="autostartEnabled"
								className="font-medium cursor-pointer"
							>
								{Language.startSwitch}
							</Label>
							{!template.allow_user_autostart && (
								<span className="text-xs text-content-secondary mt-0.5">
									The template for this workspace does not allow modification of
									autostart.
								</span>
							)}
						</div>
					</div>

					<div className="flex gap-4">
						<div className="flex flex-col gap-2 flex-1">
							<Label htmlFor="startTime">{Language.startTimeLabel}</Label>
							<Input
								id="startTime"
								name="startTime"
								type="time"
								disabled={autostartDisabled}
								value={startTimeField.value ?? ""}
								onChange={startTimeField.onChange}
								onBlur={startTimeField.onBlur}
								aria-invalid={startTimeField.error}
							/>
							{startTimeField.error && (
								<span className="text-xs text-content-destructive">
									{startTimeField.helperText}
								</span>
							)}
						</div>
						<div className="flex flex-col gap-2 flex-1">
							<Label htmlFor="timezone">{Language.timezoneLabel}</Label>
							<Select
								value={form.values.timezone}
								onValueChange={(value) => {
									void form.setFieldValue("timezone", value);
								}}
								disabled={autostartDisabled}
							>
								<SelectTrigger id="timezone">
									<SelectValue />
								</SelectTrigger>
								<SelectContent>
									{timeZones.map((zone) => (
										<SelectItem key={zone} value={zone}>
											{zone}
										</SelectItem>
									))}
								</SelectContent>
							</Select>
							{timezoneField.error && (
								<span className="text-xs text-content-destructive">
									{timezoneField.helperText}
								</span>
							)}
						</div>
					</div>

					<fieldset className="border-0 p-0 m-0">
						<legend className="text-xs text-content-secondary font-medium mb-1">
							{Language.daysOfWeekLabel}
						</legend>

						<div className="flex flex-row flex-wrap gap-x-4 gap-y-2 pt-1">
							{checkboxes.map((checkbox) => (
								<div key={checkbox.name} className="flex items-center gap-2">
									<Checkbox
										id={checkbox.name}
										checked={checkbox.value}
										disabled={
											isLoading ||
											!template.allow_user_autostart ||
											!template.autostart_requirement.days_of_week.includes(
												checkbox.name,
											) ||
											!form.values.autostartEnabled
										}
										onCheckedChange={(checked) => {
											void form.setFieldValue(checkbox.name, Boolean(checked));
										}}
									/>
									<Label htmlFor={checkbox.name} className="cursor-pointer">
										{checkbox.label}
									</Label>
								</div>
							))}
						</div>

						{form.errors.monday && (
							<span className="text-xs text-content-destructive mt-1 block">
								{Language.errorNoDayOfWeek}
							</span>
						)}
					</fieldset>
				</FormFields>
			</FormSection>

			<FormSection
				title="Autostop"
				description={
					<>
						Set how many hours should elapse after the workspace started before
						the workspace automatically shuts down. This will be extended by{" "}
						{humanDuration(template.activity_bump_ms)} after last activity in
						the workspace was detected.
					</>
				}
			>
				<FormFields>
					<div className="flex items-center gap-3">
						<Switch
							id="autostopEnabled"
							checked={form.values.autostopEnabled}
							onCheckedChange={(checked) => {
								void form.setValues({
									...form.values,
									autostopEnabled: checked,
									ttl: checked ? defaultTTL : 0,
								});
							}}
							disabled={!template.allow_user_autostop}
						/>
						<div className="flex flex-col">
							<Label
								htmlFor="autostopEnabled"
								className="font-medium cursor-pointer"
							>
								{Language.stopSwitch}
							</Label>
							{!template.allow_user_autostop && (
								<span className="text-xs text-content-secondary mt-0.5">
									The template for this workspace does not allow modification of
									autostop.
								</span>
							)}
						</div>
					</div>

					<div className="flex flex-col gap-2">
						<Label htmlFor="ttl">{Language.ttlLabel}</Label>
						<Input
							id="ttl"
							name="ttl"
							type="number"
							disabled={autostopDisabled}
							min={0}
							step="any"
							value={ttlField.value ?? ""}
							onChange={ttlField.onChange}
							onBlur={ttlField.onBlur}
							aria-invalid={ttlField.error}
						/>
						{ttlField.helperText && (
							<span
								className={
									ttlField.error
										? "text-xs text-content-destructive"
										: "text-xs text-content-secondary"
								}
							>
								{ttlField.helperText}
							</span>
						)}
					</div>
				</FormFields>
			</FormSection>

			<FormFooter>
				<Button onClick={onCancel} variant="outline">
					Cancel
				</Button>

				<Button
					type="submit"
					disabled={
						isLoading ||
						(!template.allow_user_autostart && !template.allow_user_autostop)
					}
				>
					<Spinner loading={isLoading} />
					Save
				</Button>
			</FormFooter>
		</HorizontalForm>
	);
};

export const ttlShutdownAt = (formTTL: number): string => {
	if (formTTL === 0) {
		// Passing an empty value for TTL in the form results in a number that is not zero but less than 1.
		return "Your workspace will not automatically shut down.";
	}

	try {
		return `Your workspace will shut down ${humanDuration(formTTL * 60 * 60 * 1000)} after its next start.`;
	} catch (e) {
		if (e instanceof RangeError) {
			return Language.errorTtlMax;
		}
		throw e;
	}
};
