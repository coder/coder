import type { Interpolation, Theme } from "@emotion/react";
import type { ApiErrorResponse } from "api/errors";
import type { ExternalAuthDevice } from "api/typesGenerated";
import { GitDeviceAuth } from "components/GitDeviceAuth/GitDeviceAuth";
import { SignInLayout } from "components/SignInLayout/SignInLayout";
import { Welcome } from "components/Welcome/Welcome";
import type { FC } from "react";

export interface LoginOAuthDevicePageViewProps {
	authenticated: boolean;
	redirectUrl: string;
	externalAuthDevice?: ExternalAuthDevice;
	deviceExchangeError?: ApiErrorResponse;
}

const LoginOAuthDevicePageView: FC<LoginOAuthDevicePageViewProps> = ({
	authenticated,
	redirectUrl,
	deviceExchangeError,
	externalAuthDevice,
}) => {
	if (!authenticated) {
		return (
			<SignInLayout>
				<Welcome>Authenticate with GitHub</Welcome>

				<GitDeviceAuth
					deviceExchangeError={deviceExchangeError}
					externalAuthDevice={externalAuthDevice}
				/>
			</SignInLayout>
		);
	}

	return (
		<SignInLayout>
			<Welcome>You&apos;ve authenticated with GitHub!</Welcome>

			<p css={styles.text}>
				If you&apos;re not redirected automatically,{" "}
				<a href={redirectUrl}>click here</a>.
			</p>
		</SignInLayout>
	);
};

export default LoginOAuthDevicePageView;

const styles = {
	text: (theme) => ({
		fontSize: 16,
		color: theme.palette.text.secondary,
		textAlign: "center",
		lineHeight: "160%",
		margin: 0,
	}),
} satisfies Record<string, Interpolation<Theme>>;
