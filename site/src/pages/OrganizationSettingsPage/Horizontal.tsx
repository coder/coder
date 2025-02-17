import type { Interpolation, Theme } from "@emotion/react";
import type { FC, HTMLAttributes, ReactNode } from "react";

export const HorizontalContainer: FC<HTMLAttributes<HTMLDivElement>> = ({
	...attrs
}) => {
	return <div css={styles.horizontalContainer} {...attrs} />;
};

interface HorizontalSectionProps
	extends Omit<HTMLAttributes<HTMLElement>, "title"> {
	title: ReactNode;
	description: ReactNode;
	children?: ReactNode;
}

export const HorizontalSection: FC<HorizontalSectionProps> = ({
	children,
	title,
	description,
	...attrs
}) => {
	return (
		<section css={styles.formSection} {...attrs}>
			<div css={styles.formSectionInfo}>
				<h2 css={styles.formSectionInfoTitle}>{title}</h2>
				<div css={styles.formSectionInfoDescription}>{description}</div>
			</div>

			{children}
		</section>
	);
};

const styles = {
	horizontalContainer: (theme) => ({
		display: "flex",
		flexDirection: "column",
		gap: 80,

		[theme.breakpoints.down("md")]: {
			gap: 64,
		},
	}),

	formSection: (theme) => ({
		display: "flex",
		flexDirection: "row",
		gap: 120,

		[theme.breakpoints.down("lg")]: {
			flexDirection: "column",
			gap: 16,
		},
	}),

	formSectionInfo: (theme) => ({
		width: "100%",
		flexShrink: 0,
		top: 24,
		maxWidth: 312,
		position: "sticky",

		[theme.breakpoints.down("md")]: {
			width: "100%",
			position: "initial",
		},
	}),

	formSectionInfoTitle: (theme) => ({
		fontSize: 20,
		color: theme.palette.text.primary,
		fontWeight: 400,
		margin: 0,
		marginBottom: 8,
		display: "flex",
		flexDirection: "row",
		alignItems: "center",
		gap: 12,
	}),

	formSectionInfoDescription: (theme) => ({
		fontSize: 14,
		color: theme.palette.text.secondary,
		lineHeight: "160%",
		margin: 0,
	}),
} satisfies Record<string, Interpolation<Theme>>;
