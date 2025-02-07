import type { Interpolation, Theme } from "@emotion/react";
import {
	FeatureStageBadge,
	type featureStageBadgeTypes,
} from "components/FeatureStageBadge/FeatureStageBadge";
import { Stack } from "components/Stack/Stack";
import type { FC, ReactNode } from "react";

type SectionLayout = "fixed" | "fluid";

export interface SectionProps {
	// Useful for testing
	id?: string;
	title?: ReactNode | string;
	description?: ReactNode;
	toolbar?: ReactNode;
	alert?: ReactNode;
	layout?: SectionLayout;
	className?: string;
	children?: ReactNode;
	featureStage?: keyof typeof featureStageBadgeTypes;
}

export const Section: FC<SectionProps> = ({
	id,
	title,
	description,
	toolbar,
	alert,
	className = "",
	children,
	layout = "fixed",
	featureStage,
}) => {
	return (
		<section className={className} id={id} data-testid={id}>
			<div css={{ maxWidth: layout === "fluid" ? "100%" : 500 }}>
				{(title || description) && (
					<div css={styles.header}>
						<div>
							{title && (
								<Stack direction="row" alignItems="center">
									<h4
										css={{
											fontSize: 24,
											fontWeight: 500,
											margin: 0,
											marginBottom: 8,
										}}
									>
										{title}
									</h4>
									{featureStage && (
										<FeatureStageBadge
											contentType={featureStage}
											size="lg"
											css={{ marginBottom: "5px" }}
										/>
									)}
								</Stack>
							)}
							{description && typeof description === "string" && (
								<p css={styles.description}>{description}</p>
							)}
							{description && typeof description !== "string" && (
								<div css={styles.description}>{description}</div>
							)}
						</div>
						{toolbar && <div>{toolbar}</div>}
					</div>
				)}
				{alert && <div css={{ marginBottom: 8 }}>{alert}</div>}
				{children}
			</div>
		</section>
	);
};

const styles = {
	header: {
		marginBottom: 24,
		display: "flex",
		flexDirection: "row",
		justifyContent: "space-between",
	},
	description: (theme) => ({
		color: theme.palette.text.secondary,
		fontSize: 16,
		margin: 0,
		marginTop: 4,
		lineHeight: "140%",
	}),
} satisfies Record<string, Interpolation<Theme>>;
