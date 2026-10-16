import type { AuthMethods } from "#/api/typesGenerated";
import { Alert, AlertDescription, AlertTitle } from "#/components/Alert/Alert";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { getApplicationName } from "#/utils/appearance";
import { OAuthSignInForm } from "./OAuthSignInForm";
import { PasswordSignInForm } from "./PasswordSignInForm";

type SignInFormProps = {
	isSigningIn: boolean;
	redirectTo: string;
	error?: unknown;
	message?: React.ReactNode;
	authMethods?: AuthMethods;
	onSubmit: (credentials: { email: string; password: string }) => void;
};

export const SignInForm: React.FC<SignInFormProps> = ({
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
			<h1 className="text-3xl font-semibold m-0 mb-8 leading-none">
				{applicationName}
			</h1>

			{Boolean(error) && (
				<div className="mb-8">
					<ErrorAlert error={error} showDebugDetail={false} />
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
					<AlertTitle>Sign-in isn&apos;t available</AlertTitle>
					<AlertDescription>
						This deployment has no authentication methods turned on. If you
						manage it, turn one on in the Coder server configuration.
					</AlertDescription>
				</Alert>
			)}
		</div>
	);
};
