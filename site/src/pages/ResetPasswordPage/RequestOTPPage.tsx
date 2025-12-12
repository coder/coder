import { type Interpolation, type Theme, useTheme } from "@emotion/react";
import TextField from "@mui/material/TextField";
import { requestOneTimePassword } from "api/queries/users";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { Button } from "components/Button/Button";
import { CustomLogo } from "components/CustomLogo/CustomLogo";
import { Spinner } from "components/Spinner/Spinner";
import { Stack } from "components/Stack/Stack";
import type { FC } from "react";
import { useMutation } from "react-query";
import { Link as RouterLink } from "react-router";
import { getApplicationName } from "utils/appearance";
import { pageTitle } from "utils/page";

const RequestOTPPage: FC = () => {
	const applicationName = getApplicationName();
	const requestOTPMutation = useMutation(requestOneTimePassword());

	return (
		<>
			<title>{pageTitle("Reset Password", applicationName)}</title>

			<main css={styles.root}>
				<CustomLogo css={styles.logo} />
				{requestOTPMutation.isSuccess ? (
					<RequestOTPSuccess
						email={requestOTPMutation.variables?.email ?? ""}
					/>
				) : (
					<RequestOTP
						error={requestOTPMutation.error}
						isRequesting={requestOTPMutation.isPending}
						onRequest={(email) => {
							requestOTPMutation.mutate({ email });
						}}
					/>
				)}
			</main>
		</>
	);
};

type RequestOTPProps = {
	error: unknown;
	onRequest: (email: string) => void;
	isRequesting: boolean;
};

const RequestOTP: FC<RequestOTPProps> = ({
	error,
	onRequest,
	isRequesting,
}) => {
	return (
		<div css={styles.container}>
			<div>
				<h1 className="m-0 mb-6 text-xl font-semibold leading-7">
					Enter your email to reset the password
				</h1>
				{error ? <ErrorAlert error={error} className="mb-6" /> : null}
				<form
					className="w-full"
					onSubmit={(e) => {
						e.preventDefault();
						const email = e.currentTarget.email.value;
						onRequest(email);
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
								<Button
									disabled={isRequesting}
									type="submit"
									size="lg"
									className="w-full"
								>
									<Spinner loading={isRequesting} />
									Reset password
								</Button>
								<Button asChild size="lg" variant="outline" className="w-full">
									<RouterLink to="/login">Cancel</RouterLink>
								</Button>
							</Stack>
						</Stack>
					</fieldset>
				</form>
			</div>
		</div>
	);
};

const RequestOTPSuccess: FC<{ email: string }> = ({ email }) => {
	const theme = useTheme();

	return (
		<div
			css={styles.container}
			className="max-w-[380px] font-medium text-sm leading-6"
		>
			<div>
				<p className="m-0 mb-14">
					If the account{" "}
					<span
						className="font-semibold"
						css={{ color: theme.palette.text.secondary }}
					>
						{email}
					</span>{" "}
					exists, you will get an email with instructions on resetting your
					password.
				</p>

				<p
					css={{
						color: theme.palette.text.secondary,
					}}
					className="m-0 text-xs leading-4 mb-12"
				>
					Contact your deployment administrator if you encounter issues.
				</p>

				<Button asChild variant="default">
					<RouterLink to="/login">Back to login</RouterLink>
				</Button>
			</div>
		</div>
	);
};

const styles = {
	logo: {
		marginBottom: 40,
	},
	root: {
		padding: 24,
		display: "flex",
		alignItems: "center",
		justifyContent: "center",
		flexDirection: "column",
		minHeight: "100%",
		textAlign: "center",
	},
	container: {
		width: "100%",
		maxWidth: 320,
		display: "flex",
		flexDirection: "column",
		alignItems: "center",
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

export default RequestOTPPage;
