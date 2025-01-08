import type { Interpolation, Theme } from "@emotion/react";
import { CodeExample } from "components/CodeExample/CodeExample";
import { SignInLayout } from "components/SignInLayout/SignInLayout";
import { Welcome } from "components/Welcome/Welcome";
import type { FC } from "react";
import { Link as RouterLink } from "react-router-dom";

export const CliInstallPageView: FC = () => {
	const origin = location.origin;

	return (
		<SignInLayout>
			<Welcome className="pb-3">Install the Coder CLI</Welcome>

			<p css={styles.instructions}>
				Copy the command below and{" "}
				<strong css={{ display: "block" }}>paste it in your terminal.</strong>
			</p>

			<CodeExample
				code={`curl -fsSL ${origin}/install.sh | sh`}
				secret={false}
			/>

			<div css={{ paddingTop: 16 }}>
				<RouterLink to="/workspaces" css={styles.backLink}>
					Go to workspaces
				</RouterLink>
			</div>
		</SignInLayout>
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
