import type { Interpolation, Theme } from "@emotion/react";
import { CodeExample } from "components/CodeExample/CodeExample";
import { Welcome } from "components/Welcome/Welcome";
import type { FC } from "react";
import { Link as RouterLink } from "react-router";

type CliInstallPageViewProps = {
	origin: string;
};

export const CliInstallPageView: FC<CliInstallPageViewProps> = ({ origin }) => {
	const isWindows = navigator.platform.toLowerCase().includes("win");

	return (
		<div css={styles.container}>
			<Welcome>Install the Coder CLI</Welcome>

			{isWindows ? (
				<>
					<p css={styles.instructions}>
						Download the CLI from{" "}
						<strong css={{ display: "block" }}>GitHub releases:</strong>
					</p>

					<CodeExample
						css={{ maxWidth: "100%" }}
						code="https://github.com/coder/coder/releases"
						secret={false}
					/>

					<p css={styles.windowsInstructions}>
						Download the Windows installer (.msi) or standalone binary (.exe).
						<br />
						Alternatively, use winget:
					</p>

					<CodeExample
						css={{ maxWidth: "100%" }}
						code="winget install Coder.Coder"
						secret={false}
					/>
				</>
			) : (
				<>
					<p css={styles.instructions}>
						Copy the command below and{" "}
						<strong css={{ display: "block" }}>paste it in your terminal.</strong>
					</p>

					<CodeExample
						css={{ maxWidth: "100%" }}
						code={`curl -fsSL ${origin}/install.sh | sh`}
						secret={false}
					/>
				</>
			)}

			<div css={{ paddingTop: 16 }}>
				<RouterLink to="/workspaces" css={styles.backLink}>
					Go to workspaces
				</RouterLink>
			</div>
			<div css={styles.copyright}>
				{"\u00a9"} {new Date().getFullYear()} Coder Technologies, Inc.
			</div>
		</div>
	);
};

const styles = {
	container: {
		flex: 1,
		// Fallback to 100vh
		height: ["100vh", "-webkit-fill-available"],
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

	windowsInstructions: (theme) => ({
		fontSize: 14,
		color: theme.palette.text.secondary,
		paddingTop: 16,
		paddingBottom: 8,
		textAlign: "center",
		lineHeight: 1.4,
	}),
} satisfies Record<string, Interpolation<Theme>>;
