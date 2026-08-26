import dayjs from "dayjs";
import utc from "dayjs/plugin/utc";
import type { FormikContextType } from "formik";
import { type FC, useEffect, useId, useState } from "react";
import { useNavigate } from "react-router";
import { Button } from "#/components/Button/Button";
import {
	FormFields,
	FormFooter,
	FormSection,
	HorizontalForm,
} from "#/components/Form/Form";
import { FormField } from "#/components/FormField/FormField";
import { Input } from "#/components/Input/Input";
import { Label } from "#/components/Label/Label";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "#/components/Select/Select";
import { Spinner } from "#/components/Spinner/Spinner";
import { getFormHelpers, onChangeTrimmed } from "#/utils/formUtils";
import {
	type CreateTokenData,
	customLifetimeDay,
	determineDefaultLtValue,
	filterByMaxTokenLifetime,
	NANO_HOUR,
} from "./utils";

dayjs.extend(utc);

interface CreateTokenFormProps {
	form: FormikContextType<CreateTokenData>;
	maxTokenLifetime?: number;
	formError: unknown;
	setFormError: (arg0: unknown) => void;
	isCreating: boolean;
	creationFailed: boolean;
	now?: Date;
}

export const CreateTokenForm: FC<CreateTokenFormProps> = ({
	form,
	maxTokenLifetime,
	formError,
	setFormError,
	isCreating,
	creationFailed,
	now,
}) => {
	const navigate = useNavigate();
	const lifetimeId = useId();
	const expiresOnId = useId();

	const [expDays, setExpDays] = useState<number>(1);
	const [lifetimeDays, setLifetimeDays] = useState<number | string>(
		determineDefaultLtValue(maxTokenLifetime),
	);
	const currentTime = dayjs(now ?? new Date());

	// oxlint-disable-next-line react-hooks/exhaustive-deps -- adding form will cause an infinite loop
	useEffect(() => {
		if (lifetimeDays !== "custom") {
			void form.setFieldValue("lifetime", lifetimeDays);
		} else {
			void form.setFieldValue("lifetime", expDays);
		}
	}, [lifetimeDays, expDays]);

	const getFieldHelpers = getFormHelpers<CreateTokenData>(form, formError);

	return (
		<HorizontalForm onSubmit={form.handleSubmit}>
			<FormSection
				title="Name"
				description="What is this token for?"
				classes={{ sectionInfo: "min-w-[300px]" }}
			>
				<FormFields>
					<FormField
						field={getFieldHelpers("name")}
						label="Name"
						required
						onChange={onChangeTrimmed(form, () => setFormError(undefined))}
						autoFocus
						className="w-full"
					/>
				</FormFields>
			</FormSection>
			<FormSection
				title="Expiration"
				description={
					form.values.lifetime ? (
						<>
							The token will expire on{" "}
							<span data-pixel="ignore">
								{currentTime
									.add(form.values.lifetime, "days")
									.utc()
									.format("MMMM DD, YYYY")}
							</span>
						</>
					) : (
						"Please set a token expiration."
					)
				}
				classes={{ sectionInfo: "min-w-[300px]" }}
			>
				<FormFields>
					<div className="flex flex-row gap-4">
						<div className="flex flex-col gap-2 flex-1">
							<Label htmlFor={lifetimeId}>
								Lifetime{" "}
								<span className="text-xs font-bold text-content-destructive">
									*
								</span>
							</Label>
							<Select
								value={String(lifetimeDays)}
								onValueChange={setLifetimeDays}
							>
								<SelectTrigger id={lifetimeId} className="w-full">
									<SelectValue />
								</SelectTrigger>
								<SelectContent>
									{filterByMaxTokenLifetime(maxTokenLifetime).map((lt) => (
										<SelectItem key={lt.label} value={String(lt.value)}>
											{lt.label}
										</SelectItem>
									))}
									<SelectItem value={String(customLifetimeDay.value)}>
										{customLifetimeDay.label}
									</SelectItem>
								</SelectContent>
							</Select>
						</div>

						{lifetimeDays === "custom" && (
							<div className="flex flex-col gap-2 flex-1">
								<Label htmlFor={expiresOnId}>
									Expires on{" "}
									<span className="text-xs font-bold text-content-destructive">
										*
									</span>
								</Label>
								<Input
									id={expiresOnId}
									type="date"
									data-pixel="ignore"
									defaultValue={dayjs()
										.add(expDays, "day")
										.format("YYYY-MM-DD")}
									min={dayjs().add(1, "day").format("YYYY-MM-DD")}
									max={
										maxTokenLifetime
											? dayjs()
													.add(maxTokenLifetime / NANO_HOUR / 24, "day")
													.format("YYYY-MM-DD")
											: undefined
									}
									required
									onChange={(event) => {
										const lt = Math.ceil(
											dayjs(event.target.value).diff(dayjs(), "day", true),
										);
										setExpDays(lt);
									}}
								/>
							</div>
						)}
					</div>
				</FormFields>
			</FormSection>

			<FormFooter>
				<Button onClick={() => navigate("/settings/tokens")} variant="outline">
					Cancel
				</Button>
				<Button type="submit" disabled={isCreating}>
					<Spinner loading={isCreating} />
					{creationFailed ? "Retry" : "Create token"}
				</Button>
			</FormFooter>
		</HorizontalForm>
	);
};
