import { AppWindowIcon } from "lucide-react";
import type { FC } from "react";
import { Alert } from "#/components/Alert/Alert";
import { Button } from "#/components/Button/Button";
import { SignInLayout } from "#/components/SignInLayout/SignInLayout";
import { Spinner } from "#/components/Spinner/Spinner";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { Welcome } from "#/components/Welcome/Welcome";
import { ScopeList } from "./ScopeList";
import { isUnrestricted } from "./scopes";

type OAuth2ConsentPageViewProps = {
	clientName: string;
	clientIcon?: string;
	/** Raw scope strings from the authorization request. */
	scopes: readonly string[];
	username: string;
	/** Where the user is sent back to after approving; shown so they can check it. */
	redirectUri: string;
	isSubmitting?: boolean;
	onApprove: () => void;
	onDeny: () => void;
};

/** The host is the part of the redirect URI worth reading; the path is noise. */
const redirectHost = (redirectUri: string): string => {
	try {
		return new URL(redirectUri).host;
	} catch {
		return redirectUri;
	}
};

/**
 * The OAuth2 consent screen: a third-party app has redirected the user here to
 * ask for access. This is the authorization decision point for the standard
 * web flow (Flow 2), as opposed to the device flow's confirm screen.
 *
 * Replaces the server-rendered `site/static/oauth2allow.html`, which says
 * "full access to your account" and renders no scopes at all. coder/coder#28045
 * adds a `Scopes` field to `RenderOAuthAllowData` and lists the raw strings;
 * this screen describes them instead.
 */
export const OAuth2ConsentPageView: FC<OAuth2ConsentPageViewProps> = ({
	clientName,
	clientIcon,
	scopes,
	username,
	redirectUri,
	isSubmitting = false,
	onApprove,
	onDeny,
}) => {
	const unrestricted = isUnrestricted(scopes);

	return (
		<SignInLayout>
			<main className="w-full flex flex-col gap-6">
				<div className="flex flex-col gap-2">
					{/*
					 * The heading is fixed rather than interpolating the app name: a
					 * long name wraps to several lines and pushes the decision buttons
					 * off screen. The app is identified in the card below.
					 */}
					<Welcome>Authorize access</Welcome>
					<p className="m-0 text-center text-sm text-content-secondary">
						An application is asking to use your Coder account.
					</p>
				</div>

				<div className="flex flex-col gap-4 rounded-md border border-solid border-border p-4">
					<div className="flex items-center gap-3">
						<div className="flex size-10 shrink-0 items-center justify-center overflow-hidden rounded-md border border-solid border-border bg-surface-secondary">
							{clientIcon ? (
								<img
									src={clientIcon}
									alt=""
									className="size-full object-contain p-1"
								/>
							) : (
								<AppWindowIcon
									aria-hidden="true"
									className="size-icon-sm text-content-secondary"
								/>
							)}
						</div>
						<div className="flex min-w-0 flex-col">
							<Tooltip>
								{/* Truncated rather than wrapped, full value on the tooltip. */}
								<TooltipTrigger className="block w-full truncate border-none bg-transparent p-0 text-left text-sm text-content-primary">
									{clientName}
								</TooltipTrigger>
								<TooltipContent>{clientName}</TooltipContent>
							</Tooltip>
							<span className="text-xs text-content-secondary">
								wants to act as {username}
							</span>
						</div>
					</div>

					{unrestricted ? (
						<Alert severity="warning" prominent>
							This application is asking for unrestricted access — it will be
							able to do anything you can do, including deleting workspaces and
							managing your API keys.
						</Alert>
					) : (
						<ScopeList scopes={scopes} />
					)}
				</div>

				<p className="m-0 text-center text-xs text-content-secondary">
					After approving, you'll be sent to{" "}
					<span className="font-mono">{redirectHost(redirectUri)}</span>. Only
					approve applications you trust.
				</p>

				<div className="flex flex-col gap-2">
					<Button
						className="w-full"
						disabled={isSubmitting}
						onClick={onApprove}
					>
						<Spinner loading={isSubmitting} />
						{isSubmitting ? "Authorizing…" : "Approve"}
					</Button>
					<Button
						variant="outline"
						className="w-full"
						disabled={isSubmitting}
						onClick={onDeny}
					>
						Deny
					</Button>
				</div>
			</main>
		</SignInLayout>
	);
};
