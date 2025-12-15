import { useTheme } from "@emotion/react";
import type { TemplateExample } from "api/typesGenerated";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { Button } from "components/Button/Button";
import { ExternalImage } from "components/ExternalImage/ExternalImage";
import { Loader } from "components/Loader/Loader";
import { Margins } from "components/Margins/Margins";
import { MemoizedMarkdown } from "components/Markdown/Markdown";
import {
	PageHeader,
	PageHeaderSubtitle,
	PageHeaderTitle,
} from "components/PageHeader/PageHeader";
import { Stack } from "components/Stack/Stack";
import { ExternalLinkIcon, PlusIcon } from "lucide-react";
import type { FC } from "react";
import { Link } from "react-router";

interface StarterTemplatePageViewProps {
	starterTemplate?: TemplateExample;
	error?: unknown;
}

export const StarterTemplatePageView: FC<StarterTemplatePageViewProps> = ({
	starterTemplate,
	error,
}) => {
	const theme = useTheme();

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
				<Stack direction="row" spacing={3} alignItems="center">
					<div className="size-12 flex items-center justify-center [&_img]:w-full">
						<ExternalImage src={starterTemplate.icon} />
					</div>
					<div>
						<PageHeaderTitle>{starterTemplate.name}</PageHeaderTitle>
						<PageHeaderSubtitle condensed>
							{starterTemplate.description}
						</PageHeaderSubtitle>
					</div>
				</Stack>
			</PageHeader>

			<div
				css={{
					background: theme.palette.background.paper,
					border: `1px solid ${theme.palette.divider}`,
				}}
				className="bg-content-primary border border-solid border-zinc-700 rounded-lg"
				id="readme"
			>
				<div className="pt-10 px-10 pb-16 max-w-[800px] m-auto">
					<MemoizedMarkdown>{starterTemplate.markdown}</MemoizedMarkdown>
				</div>
			</div>
		</Margins>
	);
};
