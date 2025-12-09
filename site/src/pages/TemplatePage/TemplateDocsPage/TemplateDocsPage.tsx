import { useTheme } from "@emotion/react";
import { MemoizedMarkdown } from "components/Markdown/Markdown";
import frontMatter from "front-matter";
import { useTemplateLayoutContext } from "pages/TemplatePage/TemplateLayout";

import { pageTitle } from "utils/page";

export default function TemplateDocsPage() {
	const { template, activeVersion } = useTemplateLayoutContext();
	const theme = useTheme();

	const readme = frontMatter(activeVersion.readme);

	return (
		<>
			<title>{pageTitle(template.name, "Documentation")}</title>

			<div
				css={{
					background: theme.palette.background.paper,
					border: `1px solid ${theme.palette.divider}`,
				}}
				className="rounded-lg"
				id="readme"
			>
				<div
					css={{
						color: theme.palette.text.secondary,
						borderBottom: `1px solid ${theme.palette.divider}`,
					}}
					className="font-semibold py-4 px-6"
				>
					README.md
				</div>
				<div className="pt-0 px-6 pb-10 max-w-[800px] m-auto">
					<MemoizedMarkdown>{readme.body}</MemoizedMarkdown>
				</div>
			</div>
		</>
	);
}
