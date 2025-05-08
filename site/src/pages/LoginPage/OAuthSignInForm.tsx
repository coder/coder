import Button from "@mui/material/Button";
import { visuallyHidden } from "@mui/utils";
import type { AuthMethods } from "api/typesGenerated";
import { GitHubIcon, KeyIcon } from "lucide-react";
import { type FC, useId } from "react";
import { Language } from "./Language";

const iconStyles = {
	width: 16,
	height: 16,
};

type OAuthSignInFormProps = {
	isSigningIn: boolean;
	redirectTo: string;
	authMethods?: AuthMethods;
};

export const OAuthSignInForm: FC<OAuthSignInFormProps> = ({
	isSigningIn,
	redirectTo,
	authMethods,
}) => {
	return (
		<div css={{ display: "grid", gap: "16px" }}>
			{authMethods?.github.enabled && (
				<Button
					component="a"
					href={`/api/v2/users/oauth2/github/callback?redirect=${encodeURIComponent(
						redirectTo,
					)}`}
					variant="contained"
					startIcon={<GitHubIcon css={iconStyles} />}
					disabled={isSigningIn}
					fullWidth
					type="submit"
					size="xlarge"
				>
					{Language.githubSignIn}
				</Button>
			)}

			{authMethods?.oidc.enabled && (
				<Button
					component="a"
					href={`/api/v2/users/oidc/callback?redirect=${encodeURIComponent(
						redirectTo,
					)}`}
					variant="contained"
					size="xlarge"
					startIcon={
						authMethods.oidc.iconUrl ? (
							<OidcIcon iconUrl={authMethods.oidc.iconUrl} />
						) : (
							<KeyIcon css={iconStyles} />
						)
					}
					disabled={isSigningIn}
					fullWidth
					type="submit"
				>
					{authMethods.oidc.signInText || Language.oidcSignIn}
				</Button>
			)}
		</div>
	);
};

type OidcIconProps = {
	iconUrl: string;
};

const OidcIcon: FC<OidcIconProps> = ({ iconUrl }) => {
	const hookId = useId();
	const oidcId = `${hookId}-oidc`;

	// Even if the URL is defined, there is a chance that the request for the
	// image fails. Have to use blank alt text to avoid button from getting ugly
	// if that happens, but also still need a way to inject accessible text
	return (
		<>
			<img alt="" src={iconUrl} css={iconStyles} aria-labelledby={oidcId} />
			<div id={oidcId} css={{ ...visuallyHidden }}>
				Open ID Connect
			</div>
		</>
	);
};
