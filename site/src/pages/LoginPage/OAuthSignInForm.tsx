import { visuallyHidden } from "@mui/utils";
import type { AuthMethods } from "api/typesGenerated";
import { Button } from "components/Button/Button";
import { ExternalImage } from "components/ExternalImage/ExternalImage";
import { KeyIcon } from "lucide-react";
import { type FC, useId } from "react";
import { Language } from "./Language";

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
		<div className="grid gap-4">
			{authMethods?.github.enabled && (
				<Button
					variant="outline"
					asChild
					disabled={isSigningIn}
					className="w-full"
					type="submit"
					size="lg"
				>
					<a
						href={`/api/v2/users/oauth2/github/callback?redirect=${encodeURIComponent(
							redirectTo,
						)}`}
					>
						<ExternalImage src="/icon/github.svg" />
						{Language.githubSignIn}
					</a>
				</Button>
			)}

			{authMethods?.oidc.enabled && (
				<Button
					variant="outline"
					asChild
					className="w-full"
					size="lg"
					disabled={isSigningIn}
					type="submit"
				>
					<a
						href={`/api/v2/users/oidc/callback?redirect=${encodeURIComponent(
							redirectTo,
						)}`}
					>
						{authMethods.oidc.iconUrl ? (
							<OidcIcon iconUrl={authMethods.oidc.iconUrl} />
						) : (
							<KeyIcon />
						)}
						{authMethods.oidc.signInText || Language.oidcSignIn}
					</a>
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
			<img alt="" src={iconUrl} aria-labelledby={oidcId} />
			<div id={oidcId} className="sr-only">
				Open ID Connect
			</div>
		</>
	);
};
