import type { FC } from "react";
import { Link } from "#/components/Link/Link";
import { SignInLayout } from "#/components/SignInLayout/SignInLayout";
import { Spinner } from "#/components/Spinner/Spinner";
import { Welcome } from "#/components/Welcome/Welcome";

export type OAuth2Decision = "approved" | "denied";

type OAuth2ConsentRedirectViewProps = {
	decision: OAuth2Decision;
	clientName: string;
	/** Full redirect URI, used for the manual fallback link. */
	redirectUri: string;
};

/**
 * Shown between the decision and the redirect back to the requesting app.
 *
 * Unlike the device flow, this flow ends somewhere else: the browser returns to
 * the app with an authorization code, so this screen is transient and must not
 * tell the user to close the tab. It exists so a slow or blocked redirect isn't
 * a blank page, and it always offers the link manually — the same fallback
 * `LoginOAuthDevicePageView` uses.
 */
export const OAuth2ConsentRedirectView: FC<OAuth2ConsentRedirectViewProps> = ({
	decision,
	clientName,
	redirectUri,
}) => {
	const approved = decision === "approved";

	return (
		<SignInLayout>
			<main className="w-full flex flex-col items-center gap-6">
				<Welcome>{approved ? "Access granted" : "Access not granted"}</Welcome>

				<div className="flex flex-col items-center gap-2">
					<p className="m-0 text-center text-sm text-content-secondary">
						{approved
							? `Taking you back to ${clientName}.`
							: `${clientName} was not given access. Taking you back.`}
					</p>
					<p className="flex items-center justify-center gap-2 text-sm text-content-secondary">
						<Spinner size="sm" loading />
						Redirecting…
					</p>
				</div>

				<p className="m-0 text-center text-xs text-content-secondary">
					If nothing happens, <Link href={redirectUri}>continue manually</Link>.
				</p>
			</main>
		</SignInLayout>
	);
};
