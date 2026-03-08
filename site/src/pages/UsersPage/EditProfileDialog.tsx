import TextField from "@mui/material/TextField";
import type { UpdateUserProfileRequest, User } from "api/typesGenerated";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { Button } from "components/Button/Button";
import { Dialog, DialogActionButtons } from "components/Dialogs/Dialog";
import { Form, FormFields } from "components/Form/Form";
import { Spinner } from "components/Spinner/Spinner";
import { useFormik } from "formik";
import type { FC } from "react";
import { getFormHelpers, nameValidator, onChangeTrimmed } from "utils/formUtils";
import * as Yup from "yup";

const validationSchema = Yup.object({
	username: nameValidator("Username"),
	name: Yup.string().optional(),
});

interface EditProfileDialogProps {
	user?: User;
	open: boolean;
	loading: boolean;
	error?: unknown;
	onClose: () => void;
	onConfirm: (user: User, values: UpdateUserProfileRequest) => void;
}

export const EditProfileDialog: FC<EditProfileDialogProps> = ({
	user,
	open,
	loading,
	error,
	onClose,
	onConfirm,
}) => {
	const form = useFormik<UpdateUserProfileRequest>({
		initialValues: {
			username: user?.username ?? "",
			name: user?.name ?? "",
		},
		validationSchema,
		onSubmit: (values) => {
			if (user) {
				onConfirm(user, values);
			}
		},
		enableReinitialize: true,
	});
	const getFieldHelpers = getFormHelpers(form, error);

	return (
		<Dialog
			css={{
				"& .MuiPaper-root": {
					width: "100%",
					maxWidth: 440,
				},
				"& .MuiDialogActions-spacing": {
					padding: "0 40px 40px",
				},
			}}
			onClose={onClose}
			open={open}
			data-testid="dialog"
		>
			<div
				css={(theme) => ({
					color: theme.palette.text.secondary,
					padding: "40px 40px 20px",
				})}
			>
				<h3
					css={(theme) => ({
						margin: 0,
						marginBottom: 16,
						color: theme.palette.text.primary,
						fontWeight: 400,
						fontSize: 20,
					})}
				>
					Edit profile
				</h3>
				<Form onSubmit={form.handleSubmit}>
					<FormFields>
						{Boolean(error) && <ErrorAlert error={error} />}
						<TextField
							{...getFieldHelpers("username")}
							onChange={onChangeTrimmed(form)}
							autoComplete="username"
							fullWidth
							label="Username"
						/>
						<TextField
							{...getFieldHelpers("name")}
							autoComplete="name"
							fullWidth
							onBlur={(e) => {
								e.target.value = e.target.value.trim();
								form.handleChange(e);
							}}
							label="Display Name"
						/>
						<div css={{ display: "flex", gap: 8, justifyContent: "flex-end" }}>
							<Button variant="outline" onClick={onClose} disabled={loading}>
								Cancel
							</Button>
							<Button type="submit" disabled={loading}>
								<Spinner loading={loading} />
								Save
							</Button>
						</div>
					</FormFields>
				</Form>
			</div>
		</Dialog>
	);
};
