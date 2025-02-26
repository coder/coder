import type { Interpolation, Theme } from "@emotion/react";
import OpenInNewIcon from "@mui/icons-material/OpenInNew";
import RefreshIcon from "@mui/icons-material/Refresh";
import Link from "@mui/material/Link";
import Tooltip from "@mui/material/Tooltip";
import type { ApiErrorResponse } from "api/errors";
import type { ExternalAuth, ExternalAuthDevice } from "api/typesGenerated";
import { Alert } from "components/Alert/Alert";
import { Avatar } from "components/Avatar/Avatar";
import { GitDeviceAuth } from "components/GitDeviceAuth/GitDeviceAuth";
import { SignInLayout } from "components/SignInLayout/SignInLayout";
import { Welcome } from "components/Welcome/Welcome";
import type { FC, ReactNode } from "react";

export interface ExternalAuthPageViewProps {
	externalAuth: ExternalAuth;
	viewExternalAuthConfig: boolean;

	externalAuthDevice?: ExternalAuthDevice;
	deviceExchangeError?: ApiErrorResponse;

	onReauthenticate: () => void;
}

const ExternalAuthPageView: FC<ExternalAuthPageViewProps> = ({
	deviceExchangeError,
	externalAuth,
	externalAuthDevice,
	onReauthenticate,
	viewExternalAuthConfig,
}) => {
	if (!externalAuth.authenticated) {
		return (
			<SignInLayout>
				<Welcome>Authenticate with {externalAuth.display_name}</Welcome>

				{externalAuth.device && (
					<GitDeviceAuth
						deviceExchangeError={deviceExchangeError}
						externalAuthDevice={externalAuthDevice}
					/>
				)}
			</SignInLayout>
		);
	}

	const hasInstallations = externalAuth.installations.length > 0;

	// We only want to wrap this with a link if an install URL is available!
	let installTheApp: ReactNode = `install the ${externalAuth.display_name} App`;
	if (externalAuth.app_install_url) {
		installTheApp = (
			<Link
				href={externalAuth.app_install_url}
				target="_blank"
				rel="noreferrer"
			>
				{installTheApp}
			</Link>
		);
	}

	return (
		<SignInLayout>
			<Welcome>
				You&apos;ve authenticated with {externalAuth.display_name}!
			</Welcome>

			<p css={styles.text}>
				{externalAuth.user?.login && `Hey @${externalAuth.user?.login}! 👋`}
				{(!externalAuth.app_installable ||
					externalAuth.installations.length > 0) &&
					"You are now authenticated. Feel free to close this window!"}
			</p>

			{externalAuth.installations.length > 0 && (
				<div
					css={styles.authorizedInstalls}
					className="flex gap-2 items-center"
				>
					{externalAuth.installations.map((install) => {
						if (!install.account) {
							return;
						}
						return (
							<Tooltip key={install.id} title={install.account.login}>
								<Link
									href={install.account.profile_url}
									target="_blank"
									rel="noreferrer"
								>
									<Avatar
										src={install.account.avatar_url}
										fallback={install.account.login}
									/>
								</Link>
							</Tooltip>
						);
					})}
					&nbsp;
					{externalAuth.installations.length} organization
					{externalAuth.installations.length !== 1 && "s are"} authorized
				</div>
			)}

			<div css={styles.links}>
				{!hasInstallations && externalAuth.app_installable && (
					<Alert severity="warning" css={styles.installAlert}>
						You must {installTheApp} to clone private repositories. Accounts
						will appear here once authorized.
					</Alert>
				)}

				{viewExternalAuthConfig &&
					externalAuth.app_install_url &&
					externalAuth.app_installable && (
						<Link
							href={externalAuth.app_install_url}
							target="_blank"
							rel="noreferrer"
							css={styles.link}
						>
							<OpenInNewIcon fontSize="small" />
							{externalAuth.installations.length > 0 ? "Configure" : "Install"}{" "}
							the {externalAuth.display_name} App
						</Link>
					)}
				<Link
					css={styles.link}
					href="#"
					onClick={() => {
						onReauthenticate();
					}}
				>
					<RefreshIcon /> Reauthenticate
				</Link>
			</div>
		</SignInLayout>
	);
};

export default ExternalAuthPageView;

const styles = {
	text: (theme) => ({
		fontSize: 16,
		color: theme.palette.text.secondary,
		textAlign: "center",
		lineHeight: "160%",
		margin: 0,
	}),

	installAlert: {
		margin: 16,
	},

	links: {
		display: "flex",
		gap: 4,
		margin: 16,
		flexDirection: "column",
	},

	link: {
		display: "flex",
		alignItems: "center",
		justifyContent: "center",
		fontSize: 16,
		gap: 8,
	},

	authorizedInstalls: (theme) => ({
		display: "flex",
		gap: 4,
		color: theme.palette.text.disabled,
		margin: 32,
	}),
} satisfies Record<string, Interpolation<Theme>>;
