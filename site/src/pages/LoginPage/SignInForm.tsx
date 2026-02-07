import type { AuthMethods } from "api/typesGenerated";
import { Alert } from "components/Alert/Alert";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import type { FC, ReactNode } from "react";
import { getApplicationName } from "utils/appearance";
import { OAuthSignInForm } from "./OAuthSignInForm";
import { PasswordSignInForm } from "./PasswordSignInForm";

interface SignInFormProps {
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
		<div className="w-full">
			<h1 className="text-3xl font-semibold m-0 mb-8 leading-none [&_strong]:font-semibold">
				{applicationName}
			</h1>

			{Boolean(error) && (
				<div className="mb-8">
					<ErrorAlert error={error} />
				</div>
			)}

			{message && (
				<div className="mb-8">
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
				<div className="py-6 flex items-center gap-4">
					<div className="w-full h-px bg-border" />
					<div className="shrink-0 text-content-secondary uppercase text-xs tracking-widest">
						or
					</div>
					<div className="w-full h-px bg-border" />
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
				<Alert severity="error" prominent>
					No authentication methods configured!
				</Alert>
			)}
		</div>
	);
};
