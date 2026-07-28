import { type FC, type ReactNode, useId } from "react";
import { isApiValidationError, mapApiErrorToFieldErrors } from "#/api/errors";
import type * as TypesGen from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import { Input } from "#/components/Input/Input";
import { Label } from "#/components/Label/Label";
import { Spinner } from "#/components/Spinner/Spinner";
import { cn } from "#/utils/cn";

type OAuth2AppFormValues = {
	name: string;
	callback_url: string;
	icon: string;
};

type OAuth2AppFormProps = {
	app?: TypesGen.OAuth2ProviderApp;
	onSubmit: (data: OAuth2AppFormValues) => void;
	error?: unknown;
	isUpdating: boolean;
	actions?: ReactNode;
	defaultValues?: OAuth2AppFormValues;
	disabled: boolean;
};

const formDataString = (formData: FormData, key: string): string => {
	const value = formData.get(key);
	return typeof value === "string" ? value : "";
};

type AppFormFieldProps = {
	id: string;
	name: keyof OAuth2AppFormValues;
	label: string;
	defaultValue?: string;
	errorMessage?: string;
	helperText: string;
	disabled: boolean;
	autoFocus?: boolean;
};

const AppFormField: FC<AppFormFieldProps> = ({
	id,
	name,
	label,
	defaultValue,
	errorMessage,
	helperText,
	disabled,
	autoFocus,
}) => {
	const errorId = `${id}-error`;
	const helperId = `${id}-helper`;
	const hasError = Boolean(errorMessage);

	return (
		<div className="flex flex-col gap-2">
			<Label htmlFor={id}>{label}</Label>
			<Input
				id={id}
				name={name}
				defaultValue={defaultValue}
				disabled={disabled}
				autoFocus={autoFocus}
				aria-invalid={hasError}
				aria-describedby={hasError ? errorId : helperId}
				className={cn(hasError && "border-border-destructive")}
			/>
			<span
				id={hasError ? errorId : helperId}
				className={cn(
					"text-xs",
					hasError ? "text-content-destructive" : "text-content-secondary",
				)}
			>
				{errorMessage || helperText}
			</span>
		</div>
	);
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
	const id = useId();
	const apiValidationErrors = isApiValidationError(error)
		? mapApiErrorToFieldErrors(error.response.data)
		: undefined;

	return (
		<form
			className="mt-2.5"
			onSubmit={(event) => {
				event.preventDefault();
				const formData = new FormData(event.currentTarget);
				onSubmit({
					name: formDataString(formData, "name"),
					callback_url: formDataString(formData, "callback_url"),
					icon: formDataString(formData, "icon"),
				});
			}}
		>
			<div className="flex flex-col gap-5">
				<AppFormField
					id={`${id}-name`}
					name="name"
					label="Application name"
					defaultValue={app?.name ?? defaultValues?.name}
					errorMessage={apiValidationErrors?.name}
					helperText="The name of your Coder app."
					disabled={disabled}
					autoFocus
				/>
				<AppFormField
					id={`${id}-callback-url`}
					name="callback_url"
					label="Callback URL"
					defaultValue={app?.callback_url ?? defaultValues?.callback_url}
					errorMessage={apiValidationErrors?.callback_url}
					helperText="The full URL to redirect to after a user authorizes an installation."
					disabled={disabled}
				/>
				<AppFormField
					id={`${id}-icon`}
					name="icon"
					label="Application icon"
					defaultValue={app?.icon ?? defaultValues?.icon}
					errorMessage={apiValidationErrors?.icon}
					helperText="A full or relative URL to an icon."
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
