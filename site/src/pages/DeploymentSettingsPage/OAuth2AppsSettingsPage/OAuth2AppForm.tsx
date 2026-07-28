import { useFormik } from "formik";
import type { FC, ReactNode } from "react";
import type * as TypesGen from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import { FormField } from "#/components/FormField/FormField";
import { Spinner } from "#/components/Spinner/Spinner";
import { getFormHelpers } from "#/utils/formUtils";

type OAuth2AppFormProps = {
	app?: TypesGen.OAuth2ProviderApp;
	onSubmit: (data: TypesGen.PostOAuth2ProviderAppRequest) => void;
	error?: unknown;
	isUpdating: boolean;
	actions?: ReactNode;
	defaultValues?: TypesGen.PostOAuth2ProviderAppRequest;
	disabled: boolean;
};

export const OAuth2AppForm: FC<OAuth2AppFormProps> = ({
	app,
	onSubmit,
	error,
	isUpdating,
	actions,
	defaultValues,
	disabled,
}) => {
	const form = useFormik<TypesGen.PostOAuth2ProviderAppRequest>({
		initialValues: {
			name: app?.name ?? defaultValues?.name ?? "",
			callback_url: app?.callback_url ?? defaultValues?.callback_url ?? "",
			icon: app?.icon ?? defaultValues?.icon ?? "",
		},
		// Mark fields touched from the start so server-side validation errors
		// surface as soon as they arrive instead of waiting for the user to
		// interact with each field.
		initialTouched: { name: true, callback_url: true, icon: true },
		onSubmit,
	});
	const getFieldHelpers = getFormHelpers(form, error);

	return (
		<form className="mt-2.5" onSubmit={form.handleSubmit}>
			<div className="flex flex-col gap-5">
				<FormField
					field={getFieldHelpers("name", {
						helperText: "The name of your Coder app.",
					})}
					label="Application name"
					disabled={disabled}
					autoFocus
				/>
				<FormField
					field={getFieldHelpers("callback_url", {
						helperText:
							"The full URL to redirect to after a user authorizes an installation.",
					})}
					label="Callback URL"
					disabled={disabled}
				/>
				<FormField
					field={getFieldHelpers("icon", {
						helperText: "A full or relative URL to an icon.",
					})}
					label="Application icon"
					disabled={disabled}
				/>

				<div className="flex flex-row gap-4">
					<Button disabled={isUpdating || disabled} type="submit">
						<Spinner loading={isUpdating} />
						{app ? "Update application" : "Create application"}
					</Button>
					{actions}
				</div>
			</div>
		</form>
	);
};
