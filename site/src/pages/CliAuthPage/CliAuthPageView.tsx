import type { Interpolation, Theme } from "@emotion/react";
import { visuallyHidden } from "@mui/utils";
import { CodeExample } from "components/CodeExample/CodeExample";
import { Loader } from "components/Loader/Loader";
import { Welcome } from "components/Welcome/Welcome";
import type { FC } from "react";
import { Link as RouterLink } from "react-router-dom";

export interface CliAuthPageViewProps {
	sessionToken?: string;
}

const VISUALLY_HIDDEN_SPACE = " ";

export const CliAuthPageView: FC<CliAuthPageViewProps> = ({ sessionToken }) => {
	if (!sessionToken) {
		return <Loader fullscreen />;
	}

	return (
		<div className="relative flex justify-center h-dvh flex-col items-center py-6">
			<div className="flex items-center justify-center flex-col">
				<Welcome className="pb-3">Session token</Welcome>

				<p css={styles.instructions}>
					Copy the session token below and
					{/*
					 * This looks silly, but it's a case where you want to hide the space
					 * visually because it messes up the centering, but you want the space
					 * to still be available to screen readers
					 */}
					<span css={{ ...visuallyHidden }}>{VISUALLY_HIDDEN_SPACE}</span>
					<strong css={{ display: "block" }}>paste it in your terminal.</strong>
				</p>

				<CodeExample code={sessionToken} secret />

				<div css={{ paddingTop: 16 }}>
					<RouterLink to="/workspaces" css={styles.backLink}>
						Go to workspaces
					</RouterLink>
				</div>
			</div>
			<div className="fixed bottom-6 text-xs text-content-secondary">
				{"\u00a9"} {new Date().getFullYear()} Coder Technologies, Inc.
			</div>
		</div>
	);
};

const styles = {
	instructions: (theme) => ({
		fontSize: 16,
		color: theme.palette.text.secondary,
		paddingBottom: 8,
		textAlign: "center",
		lineHeight: 1.4,

		// Have to undo styling side effects from <Welcome> component
		marginTop: -24,
	}),

	backLink: (theme) => ({
		display: "block",
		textAlign: "center",
		color: theme.palette.text.primary,
		textDecoration: "underline",
		textUnderlineOffset: 3,
		textDecorationColor: "hsla(0deg, 0%, 100%, 0.7)",
		paddingTop: 16,
		paddingBottom: 16,

		"&:hover": {
			textDecoration: "none",
		},
	}),
} satisfies Record<string, Interpolation<Theme>>;
