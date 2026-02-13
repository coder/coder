import type { AuthMethods, BuildInfoResponse } from "api/typesGenerated";
import { Button } from "components/Button/Button";
import { CustomLogo } from "components/CustomLogo/CustomLogo";
import { Loader } from "components/Loader/Loader";
import { type FC, useState } from "react";
import { useLocation } from "react-router";
import { SignInForm } from "./SignInForm";
import { TermsOfServiceLink } from "./TermsOfServiceLink";

interface LoginPageViewProps {
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
	const location = useLocation();
	// This allows messages to be displayed at the top of the sign in form.
	// Helpful for any redirects that want to inform the user of something.
	const message = new URLSearchParams(location.search).get("message");
	const [tosAccepted, setTosAccepted] = useState(false);
	const tosAcceptanceRequired =
		authMethods?.terms_of_service_url && !tosAccepted;

	return (
		<div className="p-6 flex items-center justify-center min-h-full text-center">
			<div className="w-full max-w-xs flex flex-col items-center gap-4">
				<CustomLogo />
				{isLoading ? (
					<Loader />
				) : tosAcceptanceRequired ? (
					<>
						<TermsOfServiceLink url={authMethods.terms_of_service_url} />
						<Button
							size="lg"
							className="w-full"
							onClick={() => setTosAccepted(true)}
						>
							I agree
						</Button>
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
				<footer className="text-xs text-content-secondary mt-6">
					<div>
						Copyright &copy; {new Date().getFullYear()} Coder Technologies, Inc.
					</div>
					<div>{buildInfo?.version}</div>
					{tosAccepted && (
						<TermsOfServiceLink url={authMethods?.terms_of_service_url} />
					)}
				</footer>
			</div>
		</div>
	);
};
