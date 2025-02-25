import type { Interpolation, Theme } from "@emotion/react";
import OpenInNewIcon from "@mui/icons-material/OpenInNew";
import AlertTitle from "@mui/material/AlertTitle";
import CircularProgress from "@mui/material/CircularProgress";
import Link from "@mui/material/Link";
import type { ApiErrorResponse } from "api/errors";
import type { ExternalAuthDevice } from "api/typesGenerated";
import { Alert, AlertDetail } from "components/Alert/Alert";
import { CopyButton } from "components/CopyButton/CopyButton";
import type { FC } from "react";

interface GitDeviceAuthProps {
	externalAuthDevice?: ExternalAuthDevice;
	deviceExchangeError?: ApiErrorResponse;
}

export const GitDeviceAuth: FC<GitDeviceAuthProps> = ({
	externalAuthDevice,
	deviceExchangeError,
}) => {
	let status = (
		<p css={styles.status}>
			<CircularProgress size={16} color="secondary" data-chromatic="ignore" />
			Checking for authentication...
		</p>
	);
	if (deviceExchangeError) {
		// See https://datatracker.ietf.org/doc/html/rfc8628#section-3.5
		switch (deviceExchangeError.detail) {
			case "authorization_pending":
				break;
			case "expired_token":
				status = (
					<Alert severity="error">
						The one-time code has expired. Refresh to get a new one!
					</Alert>
				);
				break;
			case "access_denied":
				status = (
					<Alert severity="error">Access to the Git provider was denied.</Alert>
				);
				break;
			default:
				status = (
					<Alert severity="error">
						<AlertTitle>{deviceExchangeError.message}</AlertTitle>
						{deviceExchangeError.detail && (
							<AlertDetail>{deviceExchangeError.detail}</AlertDetail>
						)}
					</Alert>
				);
				break;
		}
	}

	// If the error comes from the `externalAuthDevice` query,
	// we cannot even display the user_code.
	if (deviceExchangeError && !externalAuthDevice) {
		return <div>{status}</div>;
	}

	if (!externalAuthDevice) {
		return <CircularProgress />;
	}

	return (
		<div>
			<p css={styles.text}>
				Copy your one-time code:&nbsp;
				<div css={styles.copyCode}>
					<span css={styles.code}>{externalAuthDevice.user_code}</span>
					&nbsp; <CopyButton text={externalAuthDevice.user_code} />
				</div>
				<br />
				Then open the link below and paste it:
			</p>
			<div css={styles.links}>
				<Link
					css={styles.link}
					href={externalAuthDevice.verification_uri}
					target="_blank"
					rel="noreferrer"
				>
					<OpenInNewIcon fontSize="small" />
					Open and Paste
				</Link>
			</div>

			{status}
		</div>
	);
};

const styles = {
	text: (theme) => ({
		fontSize: 16,
		color: theme.palette.text.secondary,
		textAlign: "center",
		lineHeight: "160%",
		margin: 0,
	}),

	copyCode: {
		display: "inline-flex",
		alignItems: "center",
	},

	code: (theme) => ({
		fontWeight: "bold",
		color: theme.palette.text.primary,
	}),

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

	status: (theme) => ({
		display: "flex",
		alignItems: "center",
		justifyContent: "center",
		gap: 8,
		color: theme.palette.text.disabled,
	}),
} satisfies Record<string, Interpolation<Theme>>;
