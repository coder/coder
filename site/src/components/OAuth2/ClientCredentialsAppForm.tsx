import TextField from "@mui/material/TextField";
import type * as TypesGen from "api/typesGenerated";
import { Alert } from "components/Alert/Alert";
import { Button } from "components/Button/Button";
import { Spinner } from "components/Spinner/Spinner";
import { Stack } from "components/Stack/Stack";
import { useFormik } from "formik";
import { ChevronLeftIcon } from "lucide-react";
import type { FC } from "react";
import { Link as RouterLink } from "react-router-dom";
import { getFormHelpers } from "utils/formUtils";
import * as Yup from "yup";
import { Section } from "../../pages/UserSettingsPage/Section";

const validationSchema = Yup.object({
	name: Yup.string().required("Name is required"),
	icon: Yup.string(),
});

type ClientCredentialsAppFormProps = {
	app?: TypesGen.OAuth2ProviderApp;
	onSubmit: (data: {
		name: string;
		icon: string;
		grant_types: TypesGen.OAuth2ProviderGrantType[];
		redirect_uris: string[];
	}) => void;
	error?: unknown;
	isUpdating: boolean;
};

export const ClientCredentialsAppForm: FC<ClientCredentialsAppFormProps> = ({
	app,
	onSubmit,
	error,
	isUpdating,
}) => {
	const form = useFormik({
		initialValues: {
			name: app?.name || "",
			icon: app?.icon || "",
		},
		validationSchema,
		onSubmit: (values) => {
			onSubmit({
				name: values.name,
				icon: values.icon,
				grant_types: ["client_credentials"],
				redirect_uris: [], // Client credentials don't need redirect URIs
			});
		},
	});

	const getFieldHelpers = getFormHelpers(form, error);

	const title = app
		? "Edit Client Credentials Application"
		: "Create Client Credentials Application";
	const description = app
		? "Update your client credentials application details"
		: "Create a new client credentials application for server-to-server authentication";

	return (
		<>
			<Stack
				alignItems="baseline"
				direction="row"
				justifyContent="space-between"
			>
				<Section title={title} description={description} layout="fluid" />

				<Button variant="outline" asChild>
					<RouterLink to="/settings/oauth2-provider">
						<ChevronLeftIcon />
						All OAuth2 Applications
					</RouterLink>
				</Button>
			</Stack>

			<form className="mt-2.5" onSubmit={form.handleSubmit}>
				<Stack spacing={2.5}>
					<TextField
						{...getFieldHelpers("name")}
						label="Application name"
						required
						disabled={isUpdating}
						autoFocus
						fullWidth
						helperText="A descriptive name for your OAuth2 application."
					/>

					<TextField
						{...getFieldHelpers("icon")}
						label="Application icon"
						disabled={isUpdating}
						fullWidth
						helperText="A URL to an icon for your application (optional)."
					/>

					{/* Info box explaining client credentials - only show when creating */}
					{!app && (
						<Alert severity="info">
							<div>
								<div className="font-medium mb-1">
									About Client Credentials Applications
								</div>
								<div className="text-sm">
									Client credentials applications are designed for
									server-to-server communication and API access. They
									authenticate using a client ID and secret, and receive tokens
									with your user permissions. This is the recommended way to
									obtain Coder API keys for automation, providing better
									security than long-lived tokens.
								</div>
							</div>
						</Alert>
					)}

					<Stack direction="row">
						<Button disabled={isUpdating} type="submit">
							<Spinner loading={isUpdating} />
							{app ? "Update application" : "Create application"}
						</Button>
					</Stack>
				</Stack>
			</form>
		</>
	);
};
