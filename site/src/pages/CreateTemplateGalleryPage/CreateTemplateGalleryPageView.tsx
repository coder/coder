import Card from "@mui/material/Card";
import CardActionArea from "@mui/material/CardActionArea";
import CardContent from "@mui/material/CardContent";
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
import {
	StarterTemplates,
	StarterTemplatesGridContainer,
} from "./StarterTemplates";

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
							<span className="sr-only"> (link opens in new tab)</span>
						</a>
					</Button>
				}
			>
				<PageHeaderTitle>Create a Template</PageHeaderTitle>
			</PageHeader>
			<div className="flex flex-col gap-16">
				<div className="flex flex-row gap-8">
					<div className="basis-[202px] shrink-0">
						<h2 className="text-base font-normal m-0 text-content-primary">
							Choose a starting point for your new template
						</h2>
					</div>

					<StarterTemplatesGridContainer>
						<Card className="rounded-md border-border border-solid">
							<CardActionArea
								component={RouterLink}
								to="/templates/new"
								className="h-[115px] p-2"
							>
								<CardContent>
									<div className="flex flex-row gap-6 items-center">
										<div className="shrink-0 size-8">
											<ExternalImage
												src="/emojis/1f4e1.png"
												className="size-full"
											/>
										</div>
										<div>
											<h4 className="text-content-secondary text-sm font-semibold m-0 mb-1">
												Upload Template
											</h4>
											<p className="text-sm text-content-secondary m-0 leading-relaxed">
												Get started by uploading an existing template
											</p>
										</div>
									</div>
								</CardContent>
							</CardActionArea>
						</Card>
					</StarterTemplatesGridContainer>
				</div>

				{Boolean(error) && <ErrorAlert error={error} />}

				{Boolean(!starterTemplatesByTag) && <Loader />}

				<StarterTemplates starterTemplatesByTag={starterTemplatesByTag} />
			</div>
		</Margins>
	);
};
