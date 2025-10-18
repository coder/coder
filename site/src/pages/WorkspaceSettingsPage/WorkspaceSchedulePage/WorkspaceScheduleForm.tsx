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
import { Stack } from "components/Stack/Stack";
import {
	StackLabel,
	StackLabelHelperText,
} from "components/StackLabel/StackLabel";
import { Switch } from "components/Switch/Switch";
import dayjs from "dayjs";
import timezone from "dayjs/plugin/timezone";
import { type FormikTouched, useFormik } from "formik";
import {
	defaultSchedule,
	emptySchedule,
} from "pages/WorkspaceSettingsPage/WorkspaceSchedulePage/schedule";
import type { FC } from "react";
import { cn } from "utils/cn";
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

// This form utilizes complex, visually-intensive fields. Increasing the space
// between these fields enhances readability and cleanliness.
const FIELDS_SPACING = 4;

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

	const handleToggleAutostart = async (checked: boolean) => {
		if (!checked) {
			// disable autostart, clear values
			await form.setValues({
				...form.values,
				autostartEnabled: false,
				...emptySchedule,
			});
			return;
		}
		// enable autostart, fill with defaults
		await form.setValues({
			...form.values,
			autostartEnabled: true,
			...defaultSchedule(),
		});
	};

	const handleToggleAutostop = async (checked: boolean) => {
		if (!checked) {
			// disable autostop, set TTL 0
			await form.setValues({ ...form.values, autostopEnabled: false, ttl: 0 });
			return;
		}
		// enable autostop, fill with default TTL
		await form.setValues({
			...form.values,
			autostopEnabled: true,
			ttl: defaultTTL,
		});
	};

	return (
		<HorizontalForm onSubmit={form.handleSubmit}>
			<FormSection
				title="Autostart"
				description="Select the time and days of week on which you want the workspace starting automatically."
			>
				<FormFields spacing={FIELDS_SPACING}>
					<div className="flex items-center gap-2">
						<Switch
							id="autostartEnabled"
							disabled={!template.allow_user_autostart}
							name="autostartEnabled"
							checked={form.values.autostartEnabled}
							onCheckedChange={(checked) => {
								handleToggleAutostart(checked);
							}}
						/>
						<StackLabel>
							<Label htmlFor="autostartEnabled" className="cursor-pointer">
								{Language.startSwitch}
							</Label>
							{!template.allow_user_autostart && (
								<StackLabelHelperText>
									The template for this workspace does not allow modification of
									autostart.
								</StackLabelHelperText>
							)}
						</StackLabel>
					</div>
					<Stack direction="row">
						<div className="flex flex-col gap-2 w-full">
							<Label htmlFor="startTime">{Language.startTimeLabel}</Label>
							<Input
								id="startTime"
								name="startTime"
								type="time"
								value={form.values.startTime}
								onChange={form.handleChange}
								onBlur={form.handleBlur}
								// disabled if template does not allow autostart
								// or if primary feature is toggled off via the switch above
								disabled={
									isLoading ||
									!template.allow_user_autostart ||
									!form.values.autostartEnabled
								}
								aria-invalid={Boolean(
									form.errors.startTime && form.touched.startTime,
								)}
							/>
							{form.errors.startTime && form.touched.startTime && (
								<p className="text-sm text-content-danger">
									{form.errors.startTime}
								</p>
							)}
						</div>
						<div className="flex flex-col gap-2 w-full">
							<Label htmlFor="timezone">{Language.timezoneLabel}</Label>
							<Select
								name="timezone"
								value={form.values.timezone}
								onValueChange={(value) => {
									form.setFieldValue("timezone", value);
								}}
								disabled={
									isLoading ||
									!template.allow_user_autostart ||
									!form.values.autostartEnabled
								}
							>
								<SelectTrigger
									id="timezone"
									aria-label={Language.timezoneLabel}
								>
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
							{form.errors.timezone && form.touched.timezone && (
								<p className="text-sm text-content-danger">
									{form.errors.timezone}
								</p>
							)}
						</div>
					</Stack>

					<fieldset
						className={cn(
							"border-none p-0",
							form.errors.monday && "text-content-danger",
						)}
					>
						<legend className="text-xs font-medium mb-1 text-content-secondary">
							{Language.daysOfWeekLabel}
						</legend>

						<div className="flex flex-row flex-wrap gap-4 pt-1">
							{checkboxes.map((checkbox) => {
								const isAutostartDayDisabled =
									isLoading ||
									!template.allow_user_autostart ||
									!template.autostart_requirement.days_of_week.includes(
										checkbox.name,
									) ||
									!form.values.autostartEnabled;

								return (
									<div key={checkbox.name} className="flex items-center gap-2">
										<Checkbox
											id={checkbox.name}
											name={checkbox.name}
											checked={checkbox.value}
											// template admins can disable the autostart feature in general,
											// or they can disallow autostart on specific days of the week.
											// also disabled if primary feature switch (above) is toggled off
											disabled={isAutostartDayDisabled}
											onCheckedChange={(checked) => {
												form.setFieldValue(checkbox.name, checked);
											}}
										/>
										<Label
											htmlFor={checkbox.name}
											className={cn(
												"cursor-pointer text-sm",
												isAutostartDayDisabled && "text-content-disabled",
											)}
										>
											{checkbox.label}
										</Label>
									</div>
								);
							})}
						</div>

						{form.errors.monday && (
							<p className="text-sm text-content-danger mt-2">
								{Language.errorNoDayOfWeek}
							</p>
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
				<FormFields spacing={FIELDS_SPACING}>
					<div className="flex items-center gap-2">
						<Switch
							id="autostopEnabled"
							name="autostopEnabled"
							checked={form.values.autostopEnabled}
							onCheckedChange={(checked) => {
								handleToggleAutostop(checked);
							}}
							disabled={!template.allow_user_autostop}
						/>
						<StackLabel>
							<Label htmlFor="autostopEnabled" className="cursor-pointer">
								{Language.stopSwitch}
							</Label>
							{!template.allow_user_autostop && (
								<StackLabelHelperText>
									The template for this workspace does not allow modification of
									autostop.
								</StackLabelHelperText>
							)}
						</StackLabel>
					</div>
					<div className="flex flex-col gap-2 w-full">
						<Label htmlFor="ttl">{Language.ttlLabel}</Label>
						<Input
							id="ttl"
							name="ttl"
							type="number"
							value={form.values.ttl}
							onChange={form.handleChange}
							onBlur={form.handleBlur}
							// disabled if autostop disabled at template level or
							// if autostop feature is toggled off via the switch above
							disabled={
								isLoading ||
								!template.allow_user_autostop ||
								!form.values.autostopEnabled
							}
							min={0}
							step="any"
							maxLength={5}
							aria-invalid={Boolean(form.errors.ttl && form.touched.ttl)}
						/>
						<p className="text-xs text-content-secondary m-0">
							{ttlShutdownAt(form.values.ttl)}
						</p>
						{form.errors.ttl && form.touched.ttl && (
							<p className="text-sm text-content-danger">
								{typeof form.errors.ttl === "string" ? form.errors.ttl : ""}
							</p>
						)}
						{error &&
						typeof error === "object" &&
						"validations" in error &&
						Array.isArray((error as { validations?: unknown }).validations)
							? (
									(
										error as {
											validations: Array<{ field: string; detail: string }>;
										}
									).validations as Array<{ field: string; detail: string }>
								)
									.filter((v) => v.field === "ttl_ms")
									.map((v, i) => (
										<p key={i} className="text-sm text-content-danger">
											{v.detail}
										</p>
									))
							: null}
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
