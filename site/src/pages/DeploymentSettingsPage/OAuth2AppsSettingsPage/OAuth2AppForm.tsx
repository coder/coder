import TextField from "@mui/material/TextField";
import { isApiValidationError, mapApiErrorToFieldErrors } from "api/errors";
import type * as TypesGen from "api/typesGenerated";
import { Button } from "components/Button/Button";
import { Spinner } from "components/Spinner/Spinner";
import { Stack } from "components/Stack/Stack";
import type { FC, ReactNode } from "react";

type OAuth2AppFormProps = {
	app?: TypesGen.OAuth2ProviderApp;
	onSubmit: (data: {
		name: string;
		callback_url: string;
		icon: string;
	}) => void;
	error?: unknown;
	isUpdating: boolean;
	actions?: ReactNode;
	defaultValues?: {
		name: string;
		callback_url: string;
		icon: string;
	};
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
	const apiValidationErrors = isApiValidationError(error)
		? mapApiErrorToFieldErrors(error.response.data)
		: undefined;

	return (
		<form
			css={{ marginTop: 10 }}
			onSubmit={(event) => {
				event.preventDefault();
				const formData = new FormData(event.target as HTMLFormElement);
				onSubmit({
					name: formData.get("name") as string,
					callback_url: formData.get("callback_url") as string,
					icon: formData.get("icon") as string,
				});
			}}
		>
			<Stack spacing={2.5}>
				<TextField
					name="name"
					label="Application name"
					defaultValue={app?.name ?? defaultValues?.name}
					error={Boolean(apiValidationErrors?.name)}
					helperText={
						apiValidationErrors?.name || "The name of your Coder app."
					}
					disabled={disabled}
					autoFocus
					fullWidth
				/>
				<TextField
					name="callback_url"
					label="Callback URL"
					defaultValue={app?.callback_url ?? defaultValues?.callback_url}
					error={Boolean(apiValidationErrors?.callback_url)}
					helperText={
						apiValidationErrors?.callback_url ||
						"The full URL to redirect to after a user authorizes an installation."
					}
					disabled={disabled}
					fullWidth
				/>
				<TextField
					name="icon"
					label="Application icon"
					defaultValue={app?.icon ?? defaultValues?.icon}
					error={Boolean(apiValidationErrors?.icon)}
					helperText={
						apiValidationErrors?.icon || "A full or relative URL to an icon."
					}
					disabled={disabled}
					fullWidth
				/>

				<Stack direction="row">
					<Button disabled={isUpdating || disabled} type="submit">
						<Spinner loading={isUpdating} />
						{app ? "Update application" : "Create application"}
					</Button>
					{actions}
				</Stack>
			</Stack>
		</form>
	);
};
