import type { Interpolation, Theme } from "@emotion/react";
import Card from "@mui/material/Card";
import CardActionArea from "@mui/material/CardActionArea";
import CardContent from "@mui/material/CardContent";
import Stack from "@mui/material/Stack";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { ExternalImage } from "components/ExternalImage/ExternalImage";
import { Loader } from "components/Loader/Loader";
import { Margins } from "components/Margins/Margins";
import { PageHeader, PageHeaderTitle } from "components/PageHeader/PageHeader";
import type { FC } from "react";
import { Link as RouterLink } from "react-router-dom";
import type { StarterTemplatesByTag } from "utils/starterTemplates";
import { StarterTemplates } from "./StarterTemplates";

export interface CreateTemplateGalleryPageViewProps {
	starterTemplatesByTag?: StarterTemplatesByTag;
	error?: unknown;
}

export const CreateTemplateGalleryPageView: FC<
	CreateTemplateGalleryPageViewProps
> = ({ starterTemplatesByTag, error }) => {
	return (
		<Margins>
			<PageHeader>
				<PageHeaderTitle>Create a Template</PageHeaderTitle>
			</PageHeader>
			<Stack spacing={8}>
				<Stack direction="row" spacing={4}>
					<div css={{ width: 202 }}>
						<h2 css={styles.sectionTitle}>
							Choose a starting point for your new template
						</h2>
					</div>
					<div
						css={{
							display: "flex",
							flexWrap: "wrap",
							gap: 32,
							height: "max-content",
						}}
					>
						<Card variant="outlined" css={{ width: 320, borderRadius: 6 }}>
							<CardActionArea
								component={RouterLink}
								to="/templates/new"
								sx={{ height: 115, padding: 1 }}
							>
								<CardContent>
									<Stack
										direction="row"
										spacing={3}
										css={{ alignItems: "center" }}
									>
										<div css={styles.icon}>
											<ExternalImage
												src="/emojis/1f4e1.png"
												css={{
													width: "100%",
													height: "100%",
												}}
											/>
										</div>
										<div>
											<h4 css={styles.cardTitle}>Upload Template</h4>
											<span css={styles.cardDescription}>
												Get started by uploading an existing template
											</span>
										</div>
									</Stack>
								</CardContent>
							</CardActionArea>
						</Card>
					</div>
				</Stack>

				{Boolean(error) && <ErrorAlert error={error} />}

				{Boolean(!starterTemplatesByTag) && <Loader />}

				<StarterTemplates starterTemplatesByTag={starterTemplatesByTag} />
			</Stack>
		</Margins>
	);
};

const styles = {
	sectionTitle: (theme) => ({
		color: theme.palette.text.primary,
		fontSize: 16,
		fontWeight: 400,
		margin: 0,
	}),

	cardTitle: (theme) => ({
		color: theme.palette.text.secondary,
		fontSize: 14,
		fontWeight: 600,
		margin: 0,
		marginBottom: 4,
	}),

	cardDescription: (theme) => ({
		fontSize: 13,
		color: theme.palette.text.secondary,
		lineHeight: "1.6",
		display: "block",
	}),

	icon: {
		flexShrink: 0,
		width: 32,
		height: 32,
	},

	menuItemIcon: (theme) => ({
		color: theme.palette.text.secondary,
		width: 20,
		height: 20,
	}),
} satisfies Record<string, Interpolation<Theme>>;
