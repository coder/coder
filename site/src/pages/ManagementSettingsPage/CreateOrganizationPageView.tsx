import TextField from "@mui/material/TextField";
import { isApiValidationError } from "api/errors";
import type { CreateOrganizationRequest } from "api/typesGenerated";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { Badges, PremiumBadge } from "components/Badges/Badges";
import { ChooseOne, Cond } from "components/Conditionals/ChooseOne";
import {
	FormFields,
	FormFooter,
	FormSection,
	HorizontalForm,
} from "components/Form/Form";
import { IconField } from "components/IconField/IconField";
import { Paywall } from "components/Paywall/Paywall";
import { PopoverPaywall } from "components/Paywall/PopoverPaywall";
import {
	Popover,
	PopoverContent,
	PopoverTrigger,
} from "components/Popover/Popover";
import { SettingsHeader } from "components/SettingsHeader/SettingsHeader";
import { Stack } from "components/Stack/Stack";
import { useFormik } from "formik";
import type { FC } from "react";
import { docs } from "utils/docs";
import {
	displayNameValidator,
	getFormHelpers,
	nameValidator,
	onChangeTrimmed,
} from "utils/formUtils";
import * as Yup from "yup";

const MAX_DESCRIPTION_CHAR_LIMIT = 128;
const MAX_DESCRIPTION_MESSAGE = `Please enter a description that is no longer than ${MAX_DESCRIPTION_CHAR_LIMIT} characters.`;

const validationSchema = Yup.object({
	name: nameValidator("Name"),
	display_name: displayNameValidator("Display name"),
	description: Yup.string().max(
		MAX_DESCRIPTION_CHAR_LIMIT,
		MAX_DESCRIPTION_MESSAGE,
	),
});

interface CreateOrganizationPageViewProps {
	error: unknown;
	onSubmit: (values: CreateOrganizationRequest) => Promise<void>;
	isEntitled: boolean;
}

export const CreateOrganizationPageView: FC<
	CreateOrganizationPageViewProps
> = ({ error, onSubmit, isEntitled }) => {
	const form = useFormik<CreateOrganizationRequest>({
		initialValues: {
			name: "",
			display_name: "",
			description: "",
			icon: "",
		},
		validationSchema,
		onSubmit,
	});
	const getFieldHelpers = getFormHelpers(form, error);

	return (
		<Stack>
			<div>
				<SettingsHeader
					title="New Organization"
					description="Organize your deployment into multiple platform teams with unique provisioners, templates, groups, and members."
				/>

				{Boolean(error) && !isApiValidationError(error) && (
					<div css={{ marginBottom: 32 }}>
						<ErrorAlert error={error} />
					</div>
				)}

				<Badges>
					<Popover mode="hover">
						{isEntitled && (
							<PopoverTrigger>
								<span>
									<PremiumBadge />
								</span>
							</PopoverTrigger>
						)}

						<PopoverContent css={{ transform: "translateY(-28px)" }}>
							<PopoverPaywall
								message="Organizations"
								description="Create multiple organizations within a single Coder deployment, allowing several platform teams to operate with isolated users, templates, and distinct underlying infrastructure."
								documentationLink={docs("/admin/users/organizations")}
							/>
						</PopoverContent>
					</Popover>
				</Badges>
			</div>

			<ChooseOne>
				<Cond condition={!isEntitled}>
					<Paywall
						message="Organizations"
						description="Create multiple organizations within a single Coder deployment, allowing several platform teams to operate with isolated users, templates, and distinct underlying infrastructure."
						documentationLink={docs("/admin/users/organizations")}
					/>
				</Cond>
				<Cond>
					<HorizontalForm
						onSubmit={form.handleSubmit}
						aria-label="Organization settings form"
					>
						<FormSection
							title="General info"
							description="The name and description of the organization."
						>
							<fieldset
								disabled={form.isSubmitting}
								css={{
									border: "unset",
									padding: 0,
									margin: 0,
									width: "100%",
								}}
							>
								<FormFields>
									<TextField
										{...getFieldHelpers("name")}
										onChange={onChangeTrimmed(form)}
										autoFocus
										fullWidth
										label="Slug"
									/>
									<TextField
										{...getFieldHelpers("display_name")}
										fullWidth
										label="Display name"
									/>
									<TextField
										{...getFieldHelpers("description")}
										multiline
										fullWidth
										label="Description"
										rows={2}
									/>
									<IconField
										{...getFieldHelpers("icon")}
										onChange={onChangeTrimmed(form)}
										fullWidth
										onPickEmoji={(value) => form.setFieldValue("icon", value)}
									/>
								</FormFields>
							</fieldset>
						</FormSection>
						<FormFooter isLoading={form.isSubmitting} />
					</HorizontalForm>
				</Cond>
			</ChooseOne>
		</Stack>
	);
};
