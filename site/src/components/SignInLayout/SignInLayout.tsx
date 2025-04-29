import type { Interpolation, Theme } from "@emotion/react";
import { useTimeSync } from "hooks/useTimeSync";
import type { FC, PropsWithChildren } from "react";

export const SignInLayout: FC<PropsWithChildren> = ({ children }) => {
	const year = useTimeSync({
		idealRefreshIntervalMs: Number.POSITIVE_INFINITY,
		select: (date) => date.getFullYear(),
	});

	return (
		<div css={styles.container}>
			<div css={styles.content}>
				<div css={styles.signIn}>{children}</div>
				<div css={styles.copyright}>
					{"\u00a9"} {year} Coder Technologies, Inc.
				</div>
			</div>
		</div>
	);
};

const styles = {
	container: {
		flex: 1,
		height: "-webkit-fill-available",
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
