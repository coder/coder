import type { Interpolation, Theme } from "@emotion/react";
import { CodeExample } from "components/CodeExample/CodeExample";
import { Welcome } from "components/Welcome/Welcome";
import { useTimeSync } from "hooks/useTimeSync";
import type { FC } from "react";
import { Link as RouterLink } from "react-router-dom";

type CliInstallPageViewProps = {
	origin: string;
};

export const CliInstallPageView: FC<CliInstallPageViewProps> = ({ origin }) => {
	const year = useTimeSync({
		idealRefreshIntervalMs: Number.POSITIVE_INFINITY,
		select: (date) => date.getFullYear(),
	});

	return (
		<div css={styles.container}>
			<Welcome>Install the Coder CLI</Welcome>

			<p css={styles.instructions}>
				Copy the command below and{" "}
				<strong css={{ display: "block" }}>paste it in your terminal.</strong>
			</p>

			<CodeExample
				css={{ maxWidth: "100%" }}
				code={`curl -fsSL ${origin}/install.sh | sh`}
				secret={false}
			/>

			<div css={{ paddingTop: 16 }}>
				<RouterLink to="/workspaces" css={styles.backLink}>
					Go to workspaces
				</RouterLink>
			</div>
			<div css={styles.copyright}>
				{"\u00a9"} {year} Coder Technologies, Inc.
			</div>
		</div>
	);
};

const styles = {
	container: {
		flex: 1,
		height: "-webkit-fill-available",
		display: "flex",
		flexDirection: "column",
		justifyContent: "center",
		alignItems: "center",
		width: 480,
		margin: "auto",
	},

	instructions: (theme) => ({
		fontSize: 16,
		color: theme.palette.text.secondary,
		paddingBottom: 8,
		textAlign: "center",
		lineHeight: 1.4,
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

	copyright: (theme) => ({
		fontSize: 12,
		color: theme.palette.text.secondary,
		marginTop: 24,
	}),
} satisfies Record<string, Interpolation<Theme>>;
