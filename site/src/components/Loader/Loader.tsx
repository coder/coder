import type { Interpolation, Theme } from "@emotion/react";
import { Spinner } from "components/Spinner/Spinner";
import type { FC, HTMLAttributes } from "react";
import { cn } from "utils/cn";

interface LoaderProps extends HTMLAttributes<HTMLDivElement> {
	fullscreen?: boolean;
	size?: "sm" | "lg";
	/**
	 * A label for the loader. This is used for accessibility purposes.
	 */
	label?: string;
}

export const Loader: FC<LoaderProps> = ({
	fullscreen,
	size = "lg",
	label = "Loading...",
	...attrs
}) => {
	return (
		<div
			css={fullscreen ? styles.fullscreen : styles.inline}
			data-testid="loader"
			{...attrs}
		>
			<Spinner aria-label={label} size={size} loading={true} />
		</div>
	);
};

const styles = {
	inline: {
		padding: 32,
		width: "100%",
		display: "flex",
		alignItems: "center",
		justifyContent: "center",
	},
	fullscreen: (theme) => ({
		position: "absolute",
		top: "0",
		left: "0",
		right: "0",
		bottom: "0",
		display: "flex",
		justifyContent: "center",
		alignItems: "center",
		background: theme.palette.background.default,
	}),
} satisfies Record<string, Interpolation<Theme>>;
