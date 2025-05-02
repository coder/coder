import type { Interpolation, Theme } from "@emotion/react";
import type { AuthMethods } from "api/typesGenerated";
import { Alert } from "components/Alert/Alert";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import type { FC, ReactNode } from "react";
import { getApplicationName } from "utils/appearance";
import { OAuthSignInForm } from "./OAuthSignInForm";
import { PasswordSignInForm } from "./PasswordSignInForm";

const styles = {
	root: {
		width: "100%",
	},
	title: {
		fontSize: 32,
		fontWeight: 400,
		margin: 0,
		marginBottom: 32,
		lineHeight: 1,

		"& strong": {
			fontWeight: 600,
		},
	},
	alert: {
		marginBottom: 32,
	},
	divider: {
		paddingTop: 24,
		paddingBottom: 24,
		display: "flex",
		alignItems: "center",
		gap: 16,
	},
	dividerLine: (theme) => ({
		width: "100%",
		height: 1,
		backgroundColor: theme.palette.divider,
	}),
	dividerLabel: (theme) => ({
		flexShrink: 0,
		color: theme.palette.text.secondary,
		textTransform: "uppercase",
		fontSize: 12,
		letterSpacing: 1,
	}),
	icon: {
		width: 16,
		height: 16,
	},
} satisfies Record<string, Interpolation<Theme>>;

export interface SignInFormProps {
	isSigningIn: boolean;
	redirectTo: string;
	error?: unknown;
	message?: ReactNode;
	authMethods?: AuthMethods;
	onSubmit: (credentials: { email: string; password: string }) => void;
}

export const SignInForm: FC<SignInFormProps> = ({
	authMethods,
	redirectTo,
	isSigningIn,
	error,
	message,
	onSubmit,
}) => {
	const oAuthEnabled = Boolean(
		authMethods?.github.enabled || authMethods?.oidc.enabled,
	);
	const passwordEnabled = authMethods?.password.enabled ?? true;
	const applicationName = getApplicationName();

	return (
		<div css={styles.root}>
			<h1 css={styles.title}>{applicationName}</h1>

			{Boolean(error) && (
				<div css={styles.alert}>
					<ErrorAlert error={error} />
				</div>
			)}

			{message && (
				<div css={styles.alert}>
					<Alert severity="info">{message}</Alert>
				</div>
			)}

			{oAuthEnabled && (
				<OAuthSignInForm
					isSigningIn={isSigningIn}
					redirectTo={redirectTo}
					authMethods={authMethods}
				/>
			)}

			{passwordEnabled && oAuthEnabled && (
				<div css={styles.divider}>
					<div css={styles.dividerLine} />
					<div css={styles.dividerLabel}>or</div>
					<div css={styles.dividerLine} />
				</div>
			)}

			{passwordEnabled && (
				<PasswordSignInForm
					onSubmit={onSubmit}
					autoFocus={!oAuthEnabled}
					isSigningIn={isSigningIn}
				/>
			)}

			{!passwordEnabled && !oAuthEnabled && (
				<Alert severity="error">No authentication methods configured!</Alert>
			)}
		</div>
	);
};
