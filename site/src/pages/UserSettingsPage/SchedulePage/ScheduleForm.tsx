import { type FormikContextType, useFormik } from "formik";
import { type FC, useEffect, useId, useState } from "react";
import * as Yup from "yup";
import type {
	UpdateUserQuietHoursScheduleRequest,
	UserQuietHoursScheduleResponse,
} from "#/api/typesGenerated";
import { Alert } from "#/components/Alert/Alert";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Button } from "#/components/Button/Button";
import { Form, FormFields } from "#/components/Form/Form";
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
import { getFormHelpers } from "#/utils/formUtils";
import { quietHoursDisplay, timeToCron, validTime } from "#/utils/schedule";
import { getPreferredTimezone, timeZones } from "#/utils/timeZones";

interface ScheduleFormValues {
	time: string;
	timezone: string;
}

const validationSchema = Yup.object({
	time: Yup.string()
		.ensure()
		.test("is-time-string", "Time must be in HH:mm format.", (value) => {
			if (!validTime(value)) {
				return false;
			}
			const parts = value.split(":");
			const HH = Number(parts[0]);
			const mm = Number(parts[1]);
			return HH >= 0 && HH <= 23 && mm >= 0 && mm <= 59;
		}),
	timezone: Yup.string().required(),
});

interface ScheduleFormProps {
	isLoading: boolean;
	initialValues: UserQuietHoursScheduleResponse;
	submitError: unknown;
	onSubmit: (data: UpdateUserQuietHoursScheduleRequest) => void;
	// now can be set to force the time used for "Next occurrence" in tests.
	now?: Date;
}

export const ScheduleForm: FC<ScheduleFormProps> = ({
	isLoading,
	initialValues,
	submitError,
	onSubmit,
	now,
}) => {
	// Update every 15 seconds to update the "Next occurrence" field.
	const [, setTime] = useState<number>(Date.now());
	useEffect(() => {
		const interval = setInterval(() => setTime(Date.now()), 15000);
		return () => {
			clearInterval(interval);
		};
	}, []);

	// If the user has a custom schedule, use that as the initial values.
	// Otherwise, use the default time, with their local timezone.
	const formInitialValues = { ...initialValues };
	if (!initialValues.user_set) {
		formInitialValues.timezone = getPreferredTimezone();
	}

	const form: FormikContextType<ScheduleFormValues> =
		useFormik<ScheduleFormValues>({
			initialValues: formInitialValues,
			validationSchema,
			onSubmit: (values) => {
				onSubmit({
					schedule: timeToCron(values.time, values.timezone),
				});
			},
		});
	const getFieldHelpers = getFormHelpers<ScheduleFormValues>(form, submitError);
	const browserLocale = navigator.language || "en-US";
	const timezoneId = useId();
	const timezoneField = getFieldHelpers("timezone");
	const fieldsDisabled = isLoading || !initialValues.user_can_set;

	return (
		<Form onSubmit={form.handleSubmit}>
			<FormFields>
				{Boolean(submitError) && <ErrorAlert error={submitError} />}

				{!initialValues.user_set && (
					<Alert severity="info">
						You are currently using the default quiet hours schedule, which
						starts every day at <code>{initialValues.time}</code> in{" "}
						<code>{initialValues.timezone}</code>.
					</Alert>
				)}

				{!initialValues.user_can_set && (
					<Alert severity="error">
						Your administrator has disabled the ability to set a custom quiet
						hours schedule.
					</Alert>
				)}

				<div className="grid grid-cols-1 sm:grid-cols-2 items-start gap-4">
					<FormField
						field={getFieldHelpers("time")}
						label="Start time"
						type="time"
						disabled={fieldsDisabled}
						className="relative [&::-webkit-calendar-picker-indicator]:absolute [&::-webkit-calendar-picker-indicator]:right-3 [&::-webkit-calendar-picker-indicator]:cursor-pointer"
					/>
					<div className="flex flex-col gap-2 min-w-0">
						<Label htmlFor={timezoneId}>Timezone</Label>
						<Select
							value={form.values.timezone}
							onValueChange={(value) => {
								void form.setFieldValue("timezone", value);
							}}
							disabled={fieldsDisabled}
						>
							<SelectTrigger id={timezoneId}>
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
					<div className="sm:col-span-2">
						<FormField
							field={{
								name: "nextOccurrence",
								id: "nextOccurrence",
								value: quietHoursDisplay(
									browserLocale,
									form.values.time,
									form.values.timezone,
									now,
								),
								onChange: () => {},
								onBlur: () => {},
								error: false,
							}}
							label="Next occurrence"
							disabled
						/>
					</div>
				</div>

				<div className="flex justify-end">
					<Button disabled={fieldsDisabled} type="submit">
						<Spinner loading={isLoading} />
						Update schedule
					</Button>
				</div>
			</FormFields>
		</Form>
	);
};
