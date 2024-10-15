import { useTheme, type Interpolation, type Theme } from "@emotion/react";
import LoadingButton from "@mui/lab/LoadingButton";
import { Button, TextField } from "@mui/material";
import { CustomLogo } from "components/CustomLogo/CustomLogo";
import { Stack } from "components/Stack/Stack";
import type { FC } from "react";
import { Helmet } from "react-helmet-async";
import { getApplicationName } from "utils/appearance";
import { Link as RouterLink } from "react-router-dom";
import { useMutation } from "react-query";
import { requestOneTimePassword } from "api/queries/users";
import { getErrorMessage } from "api/errors";
import { displayError } from "components/GlobalSnackbar/utils";

const ResetPasswordPage: FC = () => {
	const applicationName = getApplicationName();
	const requestOTPMutation = useMutation(requestOneTimePassword());

	return (
		<>
			<Helmet>
				<title>Reset Password - {applicationName}</title>
			</Helmet>

			<div css={styles.root}>
				<main css={styles.container}>
					<CustomLogo />
					{requestOTPMutation.isSuccess ? (
						<RequestOTPSuccess
							// When requestOTPMutation.isSuccess is true,
							// requestOTPMutation.variables.email is defined
							// biome-ignore lint/style/noNonNullAssertion: Read above
							email={requestOTPMutation.variables!.email}
						/>
					) : (
						<RequestOTP
							isRequesting={requestOTPMutation.isLoading}
							onRequest={async (email) => {
								try {
									await requestOTPMutation.mutateAsync({ email });
								} catch (error) {
									displayError(
										getErrorMessage(error, "Error requesting password change"),
									);
								}
							}}
						/>
					)}
				</main>
			</div>
		</>
	);
};

const RequestOTP: FC<{
	onRequest: (email: string) => Promise<void>;
	isRequesting: boolean;
}> = ({ onRequest, isRequesting }) => {
	return (
		<>
			<h1
				css={{
					fontSize: 20,
					fontWeight: 600,
					lineHeight: "28px",
				}}
			>
				Enter your email to reset the password
			</h1>
			<form
				css={{ width: "100%" }}
				onSubmit={async (e) => {
					e.preventDefault();
					const email = e.currentTarget.email.value;
					await onRequest(email);
				}}
			>
				<fieldset disabled={isRequesting}>
					<Stack spacing={2.5}>
						<TextField
							name="email"
							label="Email"
							type="email"
							autoFocus
							required
							fullWidth
						/>

						<Stack spacing={1}>
							<LoadingButton
								loading={isRequesting}
								type="submit"
								size="large"
								fullWidth
								variant="contained"
							>
								Reset password
							</LoadingButton>
							<Button
								component={RouterLink}
								size="large"
								fullWidth
								variant="text"
								to="/login"
							>
								Cancel
							</Button>
						</Stack>
					</Stack>
				</fieldset>
			</form>
		</>
	);
};

const RequestOTPSuccess: FC<{ email: string }> = ({ email }) => {
	const theme = useTheme();

	return (
		<div
			css={{
				fontWeight: 500,
				fontSize: 14,
				lineHeight: "24px",
				maxWidth: 294,
				margin: "auto",
			}}
		>
			<p>We've sent a password reset link to the address below.</p>
			<span css={{ fontWeight: 600 }}>{email}</span>
			<p
				css={{
					fontSize: 12,
					lineHeight: "16px",
					color: theme.palette.text.secondary,
				}}
			>
				Contact your deployment administrator if you encounter issues.
			</p>
			<Button component={RouterLink} to="/login">
				Back to login
			</Button>
		</div>
	);
};

const styles = {
	root: {
		padding: 24,
		display: "flex",
		alignItems: "center",
		justifyContent: "center",
		minHeight: "100%",
		textAlign: "center",
	},
	container: {
		width: "100%",
		maxWidth: 320,
		display: "flex",
		flexDirection: "column",
		alignItems: "center",
		gap: 16,
	},
	icon: {
		fontSize: 64,
	},
	footer: (theme) => ({
		fontSize: 12,
		color: theme.palette.text.secondary,
		marginTop: 24,
	}),
} satisfies Record<string, Interpolation<Theme>>;

export default ResetPasswordPage;
