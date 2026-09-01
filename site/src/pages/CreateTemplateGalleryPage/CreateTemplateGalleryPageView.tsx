import { ExternalLinkIcon } from "lucide-react";
import type { FC } from "react";
import { Link as RouterLink } from "react-router";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Button } from "#/components/Button/Button";
import { ExternalImage } from "#/components/ExternalImage/ExternalImage";
import { Loader } from "#/components/Loader/Loader";
import { Margins } from "#/components/Margins/Margins";
import {
	PageHeader,
	PageHeaderTitle,
} from "#/components/PageHeader/PageHeader";
import type { StarterTemplatesByTag } from "#/utils/starterTemplates";
import { StarterTemplates } from "./StarterTemplates";

interface CreateTemplateGalleryPageViewProps {
	starterTemplatesByTag?: StarterTemplatesByTag;
	templateBuilderEnabled: boolean;
	error?: unknown;
}

export const CreateTemplateGalleryPageView: FC<
	CreateTemplateGalleryPageViewProps
> = ({ starterTemplatesByTag, templateBuilderEnabled, error }) => {
	return (
		<Margins className="pb-12">
			<PageHeader
				actions={
					<div className="flex flex-col items-end gap-2">
						<Button asChild size="sm" variant="outline">
							<a
								href="https://registry.coder.com/templates"
								target="_blank"
								className="flex items-center"
							>
								Browse other Templates on the Coder Registry
								<ExternalLinkIcon className="size-icon-sm ml-1" />
							</a>
						</Button>
						<Button asChild size="sm" variant="outline">
							<a
								href="https://registry.coder.com/skills/coder-templates"
								target="_blank"
								className="flex items-center"
							>
								Use our template agent skill
								<ExternalLinkIcon className="size-icon-sm ml-1" />
							</a>
						</Button>
					</div>
				}
			>
				<PageHeaderTitle>Create a Template</PageHeaderTitle>
			</PageHeader>
			<div className="flex flex-col gap-16">
				<div className="flex flex-row gap-8">
					<div className="w-[202px]">
						<h2 className="m-0 text-base font-normal text-content-primary">
							Choose a starting point for your new template
						</h2>
					</div>
					<div className="flex h-max flex-wrap gap-8">
						<RouterLink
							to="/templates/new"
							className="flex h-[115px] w-[320px] items-center gap-6 rounded-md border border-solid border-border p-4 text-inherit no-underline hover:bg-surface-secondary"
						>
							<div className="size-8 shrink-0">
								<ExternalImage
									src="/emojis/1f4e1.png"
									className="h-full w-full"
								/>
							</div>
							<div>
								<h4 className="m-0 mb-1 text-sm font-semibold text-content-secondary">
									Upload Template
								</h4>
								<span className="block text-xs font-normal leading-[1.6] text-content-secondary">
									Get started by uploading an existing template
								</span>
							</div>
						</RouterLink>
					</div>
				</div>

				{Boolean(error) && <ErrorAlert error={error} />}

				{Boolean(!starterTemplatesByTag) && <Loader />}

				<StarterTemplates
					starterTemplatesByTag={starterTemplatesByTag}
					templateBuilderEnabled={templateBuilderEnabled}
				/>
			</div>
		</Margins>
	);
};
