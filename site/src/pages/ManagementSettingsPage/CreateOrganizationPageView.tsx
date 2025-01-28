import TextField from "@mui/material/TextField";
import { isApiValidationError } from "api/errors";
import type { CreateOrganizationRequest } from "api/typesGenerated";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { Badges, PremiumBadge } from "components/Badges/Badges";
import { Button } from "components/Button/Button";
import { ChooseOne, Cond } from "components/Conditionals/ChooseOne";
import { IconField } from "components/IconField/IconField";
import { Paywall } from "components/Paywall/Paywall";
import { PopoverPaywall } from "components/Paywall/PopoverPaywall";
import { Spinner } from "components/Spinner/Spinner";
import {
	Popover,
	PopoverContent,
	PopoverTrigger,
} from "components/deprecated/Popover/Popover";
import { useFormik } from "formik";
import { ArrowLeft } from "lucide-react";
import type { FC } from "react";
import { useNavigate } from "react-router-dom";
import { Link } from "react-router-dom";
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
	const navigate = useNavigate();
	const getFieldHelpers = getFormHelpers(form, error);

	return (
		<div className="flex flex-row font-medium">
			<div className="absolute left-12">
				<Link
					to="/organizations"
					className="flex flex-row items-center gap-2 no-underline text-content-secondary hover:text-content-primary"
				>
					<ArrowLeft size={20} />
					Go Back
				</Link>
			</div>
			<div className="flex flex-col gap-4 w-full min-w-96 mx-auto">
				<div className="flex flex-col items-center">
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

					<header className="flex flex-col items-center">
						<h1 className="text-3xl font-semibold m-0">New Organization</h1>
						<p className="max-w-md text-sm text-content-secondary text-center">
							Organize your deployment into multiple platform teams with unique
							provisioners, templates, groups, and members.
						</p>
					</header>
				</div>
				<ChooseOne>
					<Cond condition={!isEntitled}>
						<div className="min-w-fit mx-auto">
							<Paywall
								message="Organizations"
								description="Create multiple organizations within a single Coder deployment, allowing several platform teams to operate with isolated users, templates, and distinct underlying infrastructure."
								documentationLink={docs("/admin/users/organizations")}
							/>
						</div>
					</Cond>
					<Cond>
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
									<TextField
										{...getFieldHelpers("name")}
										onChange={onChangeTrimmed(form)}
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
										label="Description"
										rows={2}
									/>
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
					</Cond>
				</ChooseOne>
			</div>
		</div>
	);
};
