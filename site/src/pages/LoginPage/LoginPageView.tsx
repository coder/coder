import type { Interpolation, Theme } from "@emotion/react";
import Button from "@mui/material/Button";
import type { AuthMethods, BuildInfoResponse } from "api/typesGenerated";
import { CustomLogo } from "components/CustomLogo/CustomLogo";
import { Loader } from "components/Loader/Loader";
import { useTimeSync } from "hooks/useTimeSync";
import { type FC, useState } from "react";
import { useLocation } from "react-router-dom";
import { SignInForm } from "./SignInForm";
import { TermsOfServiceLink } from "./TermsOfServiceLink";

export interface LoginPageViewProps {
	authMethods: AuthMethods | undefined;
	error: unknown;
	isLoading: boolean;
	buildInfo?: BuildInfoResponse;
	isSigningIn: boolean;
	onSignIn: (credentials: { email: string; password: string }) => void;
	redirectTo: string;
}

export const LoginPageView: FC<LoginPageViewProps> = ({
	authMethods,
	error,
	isLoading,
	buildInfo,
	isSigningIn,
	onSignIn,
	redirectTo,
}) => {
	const year = useTimeSync({
		targetRefreshInterval: Number.POSITIVE_INFINITY,
		select: (date) => date.getFullYear(),
	});
	const location = useLocation();
	// This allows messages to be displayed at the top of the sign in form.
	// Helpful for any redirects that want to inform the user of something.
	const message = new URLSearchParams(location.search).get("message");
	const [tosAccepted, setTosAccepted] = useState(false);
	const tosAcceptanceRequired =
		authMethods?.terms_of_service_url && !tosAccepted;

	return (
		<div css={styles.root}>
			<div css={styles.container}>
				<CustomLogo />
				{isLoading ? (
					<Loader />
				) : tosAcceptanceRequired ? (
					<>
						<TermsOfServiceLink url={authMethods.terms_of_service_url} />
						<Button onClick={() => setTosAccepted(true)}>I agree</Button>
					</>
				) : (
					<SignInForm
						authMethods={authMethods}
						redirectTo={redirectTo}
						isSigningIn={isSigningIn}
						error={error}
						message={message}
						onSubmit={onSignIn}
					/>
				)}
				<footer css={styles.footer}>
					<div>Copyright &copy; {year} Coder Technologies, Inc.</div>
					<div>{buildInfo?.version}</div>
					{tosAccepted && (
						<TermsOfServiceLink
							url={authMethods?.terms_of_service_url}
							css={{ fontSize: 12 }}
						/>
					)}
				</footer>
			</div>
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
