import type { Interpolation, Theme } from "@emotion/react";
import { REFRESH_IDLE, useTimeSyncState } from "hooks/useTimeSync";
import type { FC, PropsWithChildren } from "react";

export const SignInLayout: FC<PropsWithChildren> = ({ children }) => {
	const year = useTimeSyncState({
		targetIntervalMs: REFRESH_IDLE,
		transform: (d) => d.getFullYear(),
	});

	return (
		<div css={styles.container}>
			<div css={styles.content}>
				<div css={styles.signIn}>{children}</div>
				<div css={styles.copyright}>&copy;{year} Coder Technologies, Inc.</div>
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
		justifyContent: "center",
		alignItems: "center",
	},

	content: {
		display: "flex",
		flexDirection: "column",
		alignItems: "center",
	},

	signIn: {
		maxWidth: 385,
		display: "flex",
		flexDirection: "column",
		alignItems: "center",
	},

	copyright: (theme) => ({
		fontSize: 12,
		color: theme.palette.text.secondary,
		marginTop: 24,
	}),
} satisfies Record<string, Interpolation<Theme>>;
