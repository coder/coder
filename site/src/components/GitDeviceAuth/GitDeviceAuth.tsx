import type { Interpolation, Theme } from "@emotion/react";
import OpenInNewIcon from "@mui/icons-material/OpenInNew";
import AlertTitle from "@mui/material/AlertTitle";
import CircularProgress from "@mui/material/CircularProgress";
import Link from "@mui/material/Link";
import type { ApiErrorResponse } from "api/errors";
import type { ExternalAuthDevice } from "api/typesGenerated";
import { isAxiosError } from "axios";
import { Alert, AlertDetail } from "components/Alert/Alert";
import { CopyButton } from "components/CopyButton/CopyButton";
import type { FC } from "react";

interface GitDeviceAuthProps {
	externalAuthDevice?: ExternalAuthDevice;
	deviceExchangeError?: ApiErrorResponse;
}

const DeviceExchangeError = {
	AuthorizationPending: "authorization_pending",
	SlowDown: "slow_down",
	ExpiredToken: "expired_token",
	AccessDenied: "access_denied",
} as const;

export const isExchangeErrorRetryable = (_: number, error: unknown) => {
	if (!isAxiosError(error)) {
		return false;
	}
	const detail = error.response?.data?.detail;
	return (
		detail === DeviceExchangeError.AuthorizationPending ||
		detail === DeviceExchangeError.SlowDown
	);
};

/**
 * The OAuth2 specification (https://datatracker.ietf.org/doc/html/rfc8628)
 * describes how the client should handle retries. This function returns a
 * closure that implements the retry logic described in the specification.
 * The closure should be memoized because it stores state.
 */
export const newRetryDelay = (initialInterval: number | undefined) => {
	// "If no value is provided, clients MUST use 5 as the default."
	// https://datatracker.ietf.org/doc/html/rfc8628#section-3.2
	let interval = initialInterval ?? 5;
	let lastFailureCountHandled = 0;
	return (failureCount: number, error: unknown) => {
		const isSlowDown =
			isAxiosError(error) &&
			error.response?.data.detail === DeviceExchangeError.SlowDown;
		// We check the failure count to ensure we increase the interval
		// at most once per failure.
		if (isSlowDown && lastFailureCountHandled < failureCount) {
			lastFailureCountHandled = failureCount;
			// https://datatracker.ietf.org/doc/html/rfc8628#section-3.5
			// "the interval MUST be increased by 5 seconds for this and all subsequent requests"
			interval += 5;
		}
		let extraDelay = 0;
		if (isSlowDown) {
			// I found GitHub is very strict about their rate limits, and they'll block
			// even if the request is 500ms earlier than they expect. This may happen due to
			// e.g. network latency, so it's best to cool down for longer if GitHub just
			// rejected our request.
			extraDelay = 5;
		}
		return (interval + extraDelay) * 1000;
	};
};

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
			case DeviceExchangeError.AuthorizationPending:
				break;
			case DeviceExchangeError.SlowDown:
				status = (
					<div>
						{status}
						<Alert severity="warning">
							Rate limit reached. Waiting a few seconds before retrying...
						</Alert>
					</div>
				);
				break;
			case DeviceExchangeError.ExpiredToken:
				status = (
					<Alert severity="error">
						The one-time code has expired. Refresh to get a new one!
					</Alert>
				);
				break;
			case DeviceExchangeError.AccessDenied:
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
