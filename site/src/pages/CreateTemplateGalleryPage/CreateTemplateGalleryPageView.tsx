import Card from "@mui/material/Card";
import CardActionArea from "@mui/material/CardActionArea";
import CardContent from "@mui/material/CardContent";
import Stack from "@mui/material/Stack";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { Button } from "components/Button/Button";
import { ExternalImage } from "components/ExternalImage/ExternalImage";
import { Loader } from "components/Loader/Loader";
import { Margins } from "components/Margins/Margins";
import { PageHeader, PageHeaderTitle } from "components/PageHeader/PageHeader";
import { ExternalLinkIcon } from "lucide-react";
import type { FC } from "react";
import { Link as RouterLink } from "react-router";
import type { StarterTemplatesByTag } from "utils/starterTemplates";
import { StarterTemplates } from "./StarterTemplates";

interface CreateTemplateGalleryPageViewProps {
	starterTemplatesByTag?: StarterTemplatesByTag;
	error?: unknown;
}

export const CreateTemplateGalleryPageView: FC<
	CreateTemplateGalleryPageViewProps
> = ({ starterTemplatesByTag, error }) => {
	return (
		<Margins className="pb-12">
			<PageHeader
				actions={
					<Button asChild size="sm" variant="outline">
						<a
							href="https://registry.coder.com"
							target="_blank"
							rel="noopener noreferrer"
							className="flex items-center"
						>
							Browse the Coder Registry
							<ExternalLinkIcon className="size-icon-sm ml-1" />
						</a>
					</Button>
				}
			>
				<PageHeaderTitle>Create a Template</PageHeaderTitle>
			</PageHeader>
			<Stack spacing={8}>
				<Stack direction="row" spacing={4}>
					<div className="w-[202px]">
						<h2 className={classNames.sectionTitle}>
							Choose a starting point for your new template
						</h2>
					</div>
					<div className="flex flex-wrap gap-8 h-max">
						<Card variant="outlined" className="w-[320px] rounded-md">
							<CardActionArea
								component={RouterLink}
								to="/templates/new"
								sx={{ height: 115, padding: 1 }}
							>
								<CardContent>
									<Stack direction="row" spacing={3} className="items-center">
										<div className={classNames.icon}>
											<ExternalImage
												src="/emojis/1f4e1.png"
												className="w-full h-full"
											/>
										</div>
										<div>
											<h4 className={classNames.cardTitle}>Upload Template</h4>
											<span className={classNames.cardDescription}>
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

const classNames = {
	sectionTitle: "text-content-primary text-base font-normal m-0",
	cardTitle: "text-content-secondary text-sm font-medium m-0 mb-1",
	cardDescription: "text-[13px] leading-relaxed block text-content-secondary",
	icon: "flex-shrink-0 w-8 h-8",
	menuItemIcon: "text-content-secondary size-5",
};
