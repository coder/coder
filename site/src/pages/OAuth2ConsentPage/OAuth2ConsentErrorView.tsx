import type { FC } from "react";
import { Link as RouterLink } from "react-router";
import { Button } from "#/components/Button/Button";
import { SignInLayout } from "#/components/SignInLayout/SignInLayout";
import { Welcome } from "#/components/Welcome/Welcome";

/**
 * Failures that stop the flow before a decision can be made. The first three
 * mirror the errors added in coder/coder#28045
 * (`errUnknownScope`, `errScopeNotAllowed`, `errNoGrantableScope`).
 */
export type OAuth2ConsentError =
	| "unknown-scope"
	| "scope-not-allowed"
	| "no-grantable-scope"
	| "invalid-redirect"
	| "unknown-client";

type OAuth2ConsentErrorViewProps = {
	error: OAuth2ConsentError;
	clientName?: string;
	/** The offending scope, where the error is about one. */
	scope?: string;
};

/**
 * Copy is factual about what the request contained, and never blames the user —
 * they didn't compose this request, the application did. Each state says who
 * can fix it, because in every case that isn't the person reading the screen.
 */
const errorContent: Record<
	OAuth2ConsentError,
	{ title: string; body: (clientName: string, scope?: string) => string }
> = {
	"unknown-scope": {
		title: "Permission not recognized",
		body: (clientName, scope) =>
			`${clientName} asked for ${scope ? `"${scope}"` : "a permission"}, which this deployment doesn't offer. Nothing was authorized. The application's developer needs to request a supported permission.`,
	},
	"scope-not-allowed": {
		title: "Permission not allowed",
		body: (clientName, scope) =>
			`${clientName} asked for ${scope ? `"${scope}"` : "a permission"} that it isn't registered to use. Nothing was authorized. A Coder administrator can change which permissions this application may request.`,
	},
	"no-grantable-scope": {
		title: "Application needs reconfiguring",
		body: (clientName) =>
			`None of the permissions ${clientName} is registered for are supported by this deployment, so there is nothing to authorize. A Coder administrator needs to re-register it with supported permissions.`,
	},
	"invalid-redirect": {
		title: "Return address doesn't match",
		body: (clientName) =>
			`${clientName} asked to be sent to an address it isn't registered to use. Nothing was authorized, and you were not redirected — this is the check that stops an authorization being delivered somewhere it shouldn't go.`,
	},
	"unknown-client": {
		title: "Application not found",
		body: () =>
			"The application that sent you here isn't registered with this deployment. Nothing was authorized. If you expected this to work, ask a Coder administrator to register it.",
	},
};

/**
 * Terminal error for the consent flow.
 *
 * There is deliberately no "try again" — the request is malformed at the
 * source, so retrying it produces the same result. Per the Kit's error page
 * guidance the screen still offers one in-app destination rather than dead-ending.
 */
export const OAuth2ConsentErrorView: FC<OAuth2ConsentErrorViewProps> = ({
	error,
	clientName = "The application",
	scope,
}) => {
	const content = errorContent[error];

	return (
		<SignInLayout>
			<main className="w-full flex flex-col items-center gap-6">
				<Welcome>{content.title}</Welcome>

				<p className="m-0 text-center text-sm text-content-secondary">
					{content.body(clientName, scope)}
				</p>

				<Button className="w-full" asChild>
					<RouterLink to="/workspaces">Go to workspaces</RouterLink>
				</Button>
			</main>
		</SignInLayout>
	);
};
