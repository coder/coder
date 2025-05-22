import { css } from "@emotion/css";
import MenuItem from "@mui/material/MenuItem";
import TextField from "@mui/material/TextField";
import { Button } from "components/Button/Button";
import {
	FormFields,
	FormFooter,
	FormSection,
	HorizontalForm,
} from "components/Form/Form";
import { Spinner } from "components/Spinner/Spinner";
import { Stack } from "components/Stack/Stack";
import { add, differenceInDays, format } from "date-fns";
import type { FormikContextType } from "formik";
import { type FC, useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import { getFormHelpers, onChangeTrimmed } from "utils/formUtils";
import {
	type CreateTokenData,
	NANO_HOUR,
	customLifetimeDay,
	determineDefaultLtValue,
	filterByMaxTokenLifetime,
} from "./utils";

interface CreateTokenFormProps {
	form: FormikContextType<CreateTokenData>;
	maxTokenLifetime?: number;
	formError: unknown;
	setFormError: (arg0: unknown) => void;
	isCreating: boolean;
	creationFailed: boolean;
}

export const CreateTokenForm: FC<CreateTokenFormProps> = ({
	form,
	maxTokenLifetime,
	formError,
	setFormError,
	isCreating,
	creationFailed,
}) => {
	const navigate = useNavigate();

	const [expDays, setExpDays] = useState<number>(1);
	const [lifetimeDays, setLifetimeDays] = useState<number | string>(
		determineDefaultLtValue(maxTokenLifetime),
	);

	// biome-ignore lint/correctness/useExhaustiveDependencies: adding form will cause an infinite loop
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
				classes={{ sectionInfo: classNames.sectionInfo }}
			>
				<FormFields>
					<TextField
						{...getFieldHelpers("name")}
						label="Name"
						required
						onChange={onChangeTrimmed(form, () => setFormError(undefined))}
						autoFocus
						fullWidth
					/>
				</FormFields>
			</FormSection>
			<FormSection
				data-chromatic="ignore"
				title="Expiration"
				description={
					form.values.lifetime
						? `The token will expire on ${format(
								add(new Date(), { days: form.values.lifetime }),
								"MMMM dd, yyyy",
							)}`
						: "Please set a token expiration."
				}
				classes={{ sectionInfo: classNames.sectionInfo }}
			>
				<FormFields>
					<Stack direction="row">
						<TextField
							select
							label="Lifetime"
							required
							defaultValue={determineDefaultLtValue(maxTokenLifetime)}
							onChange={(event) => {
								void setLifetimeDays(event.target.value);
							}}
							fullWidth
						>
							{filterByMaxTokenLifetime(maxTokenLifetime).map((lt) => (
								<MenuItem key={lt.label} value={lt.value}>
									{lt.label}
								</MenuItem>
							))}
							<MenuItem
								key={customLifetimeDay.label}
								value={customLifetimeDay.value}
							>
								{customLifetimeDay.label}
							</MenuItem>
						</TextField>

						{lifetimeDays === "custom" && (
							<TextField
								type="date"
								label="Expires on"
								defaultValue={format(
									add(new Date(), { days: expDays }),
									"yyyy-MM-dd",
								)}
								onChange={(event) => {
									const lt = Math.ceil(
										differenceInDays(new Date(event.target.value), new Date()),
									);
									setExpDays(lt);
								}}
								inputProps={{
									"data-chromatic": "ignore",
									min: format(add(new Date(), { days: 1 }), "yyyy-MM-dd"),
									max: maxTokenLifetime
										? format(
												add(new Date(), {
													days: maxTokenLifetime / NANO_HOUR / 24,
												}),
												"yyyy-MM-dd",
											)
										: undefined,
									required: true,
								}}
								fullWidth
								InputLabelProps={{
									required: true,
								}}
							/>
						)}
					</Stack>
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

const classNames = {
	sectionInfo: css`
    min-width: 300px;
  `,
};
