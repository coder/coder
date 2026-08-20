import { useFormik } from "formik";
import { ArrowLeftIcon } from "lucide-react";
import type { FC } from "react";
import { useMutation, useQueryClient } from "react-query";
import { Link, useNavigate } from "react-router";
import { toast } from "sonner";
import * as Yup from "yup";
import { getErrorMessage, isApiValidationError } from "#/api/errors";
import { createOrganization } from "#/api/queries/organizations";
import type { CreateOrganizationRequest } from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Button } from "#/components/Button/Button";
import { FormField } from "#/components/FormField/FormField";
import { IconField } from "#/components/IconField/IconField";
import { Label } from "#/components/Label/Label";
import { PaywallPremium } from "#/components/Paywall/PaywallPremium";
import {
	SettingsHeader,
	SettingsHeaderDescription,
	SettingsHeaderTitle,
} from "#/components/SettingsHeader/SettingsHeader";
import { Spinner } from "#/components/Spinner/Spinner";
import { Textarea } from "#/components/Textarea/Textarea";
import type { Permissions } from "#/modules/permissions";
import { cn } from "#/utils/cn";
import {
	displayNameValidator,
	getFormHelpers,
	nameValidator,
	onChangeTrimmed,
} from "#/utils/formUtils";

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
	isEntitled: boolean;
	permissions: Permissions;
}

export const CreateOrganizationPageView: FC<
	CreateOrganizationPageViewProps
> = ({ isEntitled, permissions }) => {
	const navigate = useNavigate();
	const queryClient = useQueryClient();
	const createOrganizationMutation = useMutation(
		createOrganization(queryClient),
	);
	const error = createOrganizationMutation.error;

	const form = useFormik<CreateOrganizationRequest>({
		initialValues: {
			name: "",
			display_name: "",
			description: "",
			icon: "",
		},
		validationSchema,
		onSubmit: async (values) => {
			try {
				await createOrganizationMutation.mutateAsync(values);
				toast.success(`Organization "${values.name}" created successfully.`);
				await navigate(`/organizations/${values.name}`);
			} catch (err) {
				toast.error(
					getErrorMessage(
						err,
						`Failed to create organization "${values.name}".`,
					),
				);
			}
		},
	});
	const getFieldHelpers = getFormHelpers(form, error);
	const descriptionField = getFieldHelpers("description", {
		maxLength: MAX_DESCRIPTION_CHAR_LIMIT,
		helperText: "Optional. Short summary of this organization.",
	});
	const iconField = getFieldHelpers("icon", {
		helperText: "Optional. URL or emoji shown for this organization.",
	});
	const descriptionErrorId = `${descriptionField.id}-error`;
	const descriptionHelperId = `${descriptionField.id}-helper`;

	return (
		<section className="px-4 sm:px-6 lg:px-10 py-6 lg:py-10">
			<div className="flex flex-col gap-4 w-full mx-auto max-w-4xl">
				<Button variant="subtle" asChild className="-ml-3 self-start">
					<Link to="/organizations">
						<ArrowLeftIcon />
						<span>Back to organizations</span>
					</Link>
				</Button>

				<div className="flex flex-col">
					<SettingsHeader>
						<SettingsHeaderTitle>New Organization</SettingsHeaderTitle>
						<SettingsHeaderDescription>
							Isolate members, templates, and provisioners for a team or
							project.
						</SettingsHeaderDescription>
					</SettingsHeader>

					{!isEntitled ? (
						<PaywallPremium
							message="Organizations"
							description="Isolate members, templates, and provisioners for a team or project within a single Coder deployment."
							canViewPremium={permissions.viewAllLicenses}
						/>
					) : (
						<div className="border border-solid p-6 rounded-lg">
							<form
								onSubmit={form.handleSubmit}
								aria-label="Organization settings form"
								className="flex flex-col gap-6 w-full"
							>
								{Boolean(error) && !isApiValidationError(error) && (
									<ErrorAlert error={error} />
								)}
								<fieldset
									disabled={form.isSubmitting}
									className="flex flex-col gap-6 w-full border-none p-0 m-0"
								>
									<div className="grid grid-cols-1 sm:grid-cols-2 items-start gap-4">
										<FormField
											field={getFieldHelpers("name", {
												helperText: "Unique identifier used in URLs.",
											})}
											label="Slug"
											required
											className="w-full"
											onChange={onChangeTrimmed(form)}
										/>
										<FormField
											field={getFieldHelpers("display_name", {
												helperText:
													"Friendly name. Defaults to the slug if blank.",
											})}
											label="Display name"
											className="w-full"
										/>
									</div>
									<div className="flex flex-col gap-2">
										<Label htmlFor={descriptionField.id}>Description</Label>
										<Textarea
											id={descriptionField.id}
											name={descriptionField.name}
											value={descriptionField.value}
											onChange={descriptionField.onChange}
											onBlur={descriptionField.onBlur}
											rows={2}
											aria-invalid={descriptionField.error}
											aria-describedby={
												descriptionField.error
													? descriptionErrorId
													: descriptionField.helperText
														? descriptionHelperId
														: undefined
											}
											className={cn(
												"resize-none",
												descriptionField.error && "border-border-destructive",
											)}
										/>
										{descriptionField.error ? (
											<span
												id={descriptionErrorId}
												className="text-xs text-content-destructive"
											>
												{descriptionField.helperText}
											</span>
										) : (
											descriptionField.helperText && (
												<span
													id={descriptionHelperId}
													className="text-xs text-content-secondary"
												>
													{descriptionField.helperText}
												</span>
											)
										)}
									</div>
									<IconField
										{...iconField}
										disabled={form.isSubmitting}
										onChange={onChangeTrimmed(form)}
										onPickEmoji={(value) => {
											void form.setFieldValue("icon", value);
											void form.setFieldTouched("icon", true);
										}}
									/>
								</fieldset>
								<div className="flex justify-end gap-4">
									<Button asChild variant="outline">
										<Link to="/organizations">Cancel</Link>
									</Button>
									<Button type="submit" disabled={form.isSubmitting}>
										<Spinner loading={form.isSubmitting} />
										Create organization
									</Button>
								</div>
							</form>
						</div>
					)}
				</div>
			</div>
		</section>
	);
};
