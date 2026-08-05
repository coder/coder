import { useFormik } from "formik";
import { ArrowLeftIcon } from "lucide-react";
import type { FC } from "react";
import { Link, useNavigate } from "react-router";
import * as Yup from "yup";
import { isApiValidationError } from "#/api/errors";
import type { CreateOrganizationRequest } from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Badges, PremiumBadge } from "#/components/Badges/Badges";
import { Button } from "#/components/Button/Button";
import { FormField } from "#/components/FormField/FormField";
import { IconField } from "#/components/IconField/IconField";
import { Label } from "#/components/Label/Label";
import { PaywallPremium } from "#/components/Paywall/PaywallPremium";
import { Spinner } from "#/components/Spinner/Spinner";
import { Textarea } from "#/components/Textarea/Textarea";
import type { Permissions } from "#/modules/permissions";
import { cn } from "#/utils/cn";
import { docs } from "#/utils/docs";
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
	error: unknown;
	onSubmit: (values: CreateOrganizationRequest) => Promise<void>;
	isEntitled: boolean;
	permissions: Permissions;
}

export const CreateOrganizationPageView: FC<
	CreateOrganizationPageViewProps
> = ({ error, onSubmit, isEntitled, permissions }) => {
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
	const navigate = useNavigate();
	const getFieldHelpers = getFormHelpers(form, error);
	const descriptionField = getFieldHelpers("description", {
		maxLength: MAX_DESCRIPTION_CHAR_LIMIT,
	});
	const descriptionErrorId = `${descriptionField.id}-error`;
	const descriptionHelperId = `${descriptionField.id}-helper`;

	return (
		<div className="flex flex-row font-medium">
			<div className="absolute left-12">
				<Link
					to="/organizations"
					className="flex flex-row items-center gap-2 no-underline text-content-secondary hover:text-content-primary"
				>
					<ArrowLeftIcon size={20} />
					Go Back
				</Link>
			</div>
			<div className="flex flex-col gap-4 w-full min-w-96 mx-auto">
				<div className="flex flex-col items-center">
					{Boolean(error) && !isApiValidationError(error) && (
						<div className="mb-8">
							<ErrorAlert error={error} />
						</div>
					)}

					{isEntitled && (
						<Badges>
							<PremiumBadge />
						</Badges>
					)}

					<header className="flex flex-col items-center">
						<h1 className="text-3xl font-semibold m-0">New Organization</h1>
						<p className="max-w-md text-sm text-content-secondary text-center">
							Organize your deployment into multiple platform teams with unique
							provisioners, templates, groups, and members.
						</p>
					</header>
				</div>
				{!isEntitled ? (
					<div className="min-w-fit mx-auto">
						<PaywallPremium
							message="Organizations"
							description="Create multiple organizations within a single Coder deployment, allowing several platform teams to operate with isolated users, templates, and distinct underlying infrastructure."
							documentationLink={docs("/admin/users/organizations")}
							canViewPremium={permissions.viewAllLicenses}
						/>
					</div>
				) : (
					<div className="flex flex-col gap-4 w-full max-w-xl min-w-72 mx-auto">
						<form
							onSubmit={form.handleSubmit}
							aria-label="Organization settings form"
							className="flex flex-col gap-6 w-full"
						>
							<fieldset
								disabled={form.isSubmitting}
								className="flex flex-col gap-6 w-full border-none"
							>
								<FormField
									field={getFieldHelpers("name")}
									label="Slug"
									onChange={onChangeTrimmed(form)}
								/>
								<FormField
									field={getFieldHelpers("display_name")}
									label="Display name"
								/>
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
									{...getFieldHelpers("icon")}
									onChange={onChangeTrimmed(form)}
									onPickEmoji={(value) => form.setFieldValue("icon", value)}
								/>
							</fieldset>
							<div className="flex flex-row gap-2">
								<Button type="submit" disabled={form.isSubmitting}>
									{form.isSubmitting && <Spinner />}
									Save
								</Button>
								<Button
									variant="outline"
									type="button"
									onClick={() => navigate("/organizations")}
								>
									Cancel
								</Button>
							</div>
						</form>
					</div>
				)}
			</div>
		</div>
	);
};
