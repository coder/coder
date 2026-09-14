import { ExternalLinkIcon, PlusIcon } from "lucide-react";
import type { FC } from "react";
import { Link } from "react-router";
import type { TemplateExample } from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Button } from "#/components/Button/Button";
import { ExternalImage } from "#/components/ExternalImage/ExternalImage";
import { Loader } from "#/components/Loader/Loader";
import { Margins } from "#/components/Margins/Margins";
import { MemoizedMarkdown } from "#/components/Markdown/Markdown";
import {
	PageHeader,
	PageHeaderSubtitle,
	PageHeaderTitle,
} from "#/components/PageHeader/PageHeader";

interface StarterTemplatePageViewProps {
	starterTemplate?: TemplateExample;
	error?: unknown;
}

export const StarterTemplatePageView: FC<StarterTemplatePageViewProps> = ({
	starterTemplate,
	error,
}) => {
	if (error) {
		return (
			<Margins>
				<ErrorAlert error={error} />
			</Margins>
		);
	}

	if (!starterTemplate) {
		return <Loader />;
	}

	return (
		<Margins>
			<PageHeader
				actions={
					<>
						<Button asChild variant="outline" size="sm">
							<a target="_blank" href={starterTemplate.url} rel="noreferrer">
								<ExternalLinkIcon />
								View source code
							</a>
						</Button>
						<Button asChild size="sm">
							<Link to={`/templates/new?exampleId=${starterTemplate.id}`}>
								<PlusIcon />
								Use template
							</Link>
						</Button>
					</>
				}
			>
				<div className="flex flex-row gap-6 items-center">
					<div className="size-12 flex items-center justify-center [&_img]:w-full">
						<ExternalImage src={starterTemplate.icon} />
					</div>
					<div>
						<PageHeaderTitle>{starterTemplate.name}</PageHeaderTitle>
						<PageHeaderSubtitle>
							{starterTemplate.description}
						</PageHeaderSubtitle>
					</div>
				</div>
			</PageHeader>

			<div
				className="bg-surface-secondary border border-solid border-border rounded-lg"
				id="readme"
			>
				<div className="px-8 pt-2 pb-12 max-w-[860px]">
					<MemoizedMarkdown>{starterTemplate.markdown}</MemoizedMarkdown>
				</div>
			</div>
		</Margins>
	);
};
