import { type FC, useState } from "react";
import { useSearchParams } from "react-router";
import { pageTitle } from "#/utils/page";
import { OAuth2ConsentPageView } from "./OAuth2ConsentPageView";
import {
	OAuth2ConsentRedirectView,
	type OAuth2Decision,
} from "./OAuth2ConsentRedirectView";

/**
 * OAuth2 authorization consent page (Flow 2 — PLAT-470 / PLAT-479).
 *
 * A third-party app redirects the user here with `client_id`, `scope` and
 * `redirect_uri`; the user approves or denies; on approval the browser is sent
 * back to the app with an authorization code.
 *
 * TODO(oauth2-consent): this is a sketch. Today the same URL is served by the
 * Go template `site/static/oauth2allow.html` via `ShowAuthorizePage`
 * (`coderd/oauth2provider/authorize.go`), so adopting this screen means the
 * backend serving the SPA at `/oauth2/authorize` and exposing the app record —
 * name, icon, requested scopes, redirect URI — to the frontend. The scope
 * plumbing is being added in coder/coder#28045.
 */
const OAuth2ConsentPage: FC = () => {
	const [searchParams] = useSearchParams();
	const [decision, setDecision] = useState<OAuth2Decision>();

	// Placeholders until the app record is available to the frontend.
	const clientName = "Application";
	const username = "";
	const scopes = (searchParams.get("scope") ?? "").split(" ").filter(Boolean);
	const redirectUri = searchParams.get("redirect_uri") ?? "";

	return (
		<>
			<title>{pageTitle("Authorize access")}</title>

			{decision ? (
				<OAuth2ConsentRedirectView
					decision={decision}
					clientName={clientName}
					redirectUri={redirectUri}
				/>
			) : (
				<OAuth2ConsentPageView
					clientName={clientName}
					scopes={scopes}
					username={username}
					redirectUri={redirectUri}
					onApprove={() => setDecision("approved")}
					onDeny={() => setDecision("denied")}
				/>
			)}
		</>
	);
};

export default OAuth2ConsentPage;
