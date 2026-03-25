import { isApiValidationError } from "api/errors";
import type { CreateGroupRequest } from "api/typesGenerated";
import { useFormik } from "formik";
import type { FC } from "react";
import { useNavigate } from "react-router";
import {
	getFormHelpers,
	nameValidator,
	onChangeTrimmed,
} from "utils/formUtils";
import * as Yup from "yup";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Button } from "#/components/Button/Button";
import { IconField } from "#/components/IconField/IconField";
import { Input } from "#/components/Input/Input";
import { Label } from "#/components/Label/Label";
import { Spinner } from "#/components/Spinner/Spinner";

const validationSchema = Yup.object({
	name: nameValidator("Name"),
});

type CreateGroupPageViewProps = {
	onSubmit: (data: CreateGroupRequest) => void;
	error?: unknown;
	isLoading: boolean;
};

export const CreateGroupPageView: FC<CreateGroupPageViewProps> = ({
	onSubmit,
	error,
	isLoading,
}) => {
	const navigate = useNavigate();
	const form = useFormik<CreateGroupRequest>({
		initialValues: {
			name: "",
			display_name: "",
			avatar_url: "",
			quota_allowance: 0,
		},
		validationSchema,
		onSubmit,
	});
	const getFieldHelpers = getFormHelpers<CreateGroupRequest>(form, error);
	const onCancel = () => navigate(-1);
	const nameField = getFieldHelpers("name");
	const displayNameField = getFieldHelpers("display_name", {
		helperText: "Keep empty to default to the name.",
	});

	return (
		<div className="flex flex-col items-start w-full max-w-xl">
			<div className="flex flex-row items-start pb-6">
				<h1 className="m-0 flex items-center gap-2 text-3xl font-semibold leading-tight">
					New Group
				</h1>
			</div>

			<form
				className="flex flex-col w-full max-w-xl  gap-10 rounded-lg border border-solid border-border-default p-6"
				onSubmit={form.handleSubmit}
			>
				<section className="flex flex-col gap-4">
					<div className="flex flex-col gap-2">
						<h2 className="text-xl font-medium text-content-primary m-0">
							Group settings
						</h2>
						<p className="text-sm leading-relaxed text-content-secondary m-0">
							Set a name and avatar for this group.
						</p>
					</div>
					<div className="flex flex-col gap-6">
						{Boolean(error) && !isApiValidationError(error) && (
							<ErrorAlert error={error} />
						)}

						<div className="flex flex-col items-start gap-2">
							<Label htmlFor={nameField.id}>Name</Label>
							<Input
								id={nameField.id}
								name={nameField.name}
								value={nameField.value}
								onChange={onChangeTrimmed(form)}
								onBlur={nameField.onBlur}
								autoFocus
								autoComplete="name"
								aria-invalid={nameField.error}
							/>
							{nameField.helperText && (
								<span
									className={`text-xs text-left ${
										nameField.error
											? "text-content-destructive"
											: "text-content-secondary"
									}`}
								>
									{nameField.helperText}
								</span>
							)}
						</div>
						<div className="flex flex-col items-start gap-2">
							<Label htmlFor={displayNameField.id}>Display Name</Label>
							<Input
								id={displayNameField.id}
								name={displayNameField.name}
								value={displayNameField.value}
								onChange={displayNameField.onChange}
								onBlur={displayNameField.onBlur}
								autoComplete="display_name"
								aria-invalid={displayNameField.error}
							/>
							{displayNameField.helperText && (
								<span
									className={`text-xs text-left ${
										displayNameField.error
											? "text-content-destructive"
											: "text-content-secondary"
									}`}
								>
									{displayNameField.helperText}
								</span>
							)}
						</div>
						<IconField
							{...getFieldHelpers("avatar_url")}
							onChange={onChangeTrimmed(form)}
							fullWidth
							label="Avatar URL"
							onPickEmoji={(value) => form.setFieldValue("avatar_url", value)}
						/>
					</div>
				</section>

				<footer className="flex items-center justify-end space-x-2">
					<Button onClick={onCancel} variant="outline">
						Cancel
					</Button>
					<Button type="submit" disabled={isLoading}>
						<Spinner loading={isLoading} />
						Create group
					</Button>
				</footer>
			</form>
		</div>
	);
};
