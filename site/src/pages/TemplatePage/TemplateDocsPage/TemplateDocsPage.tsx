import { MemoizedMarkdown } from "components/Markdown/Markdown";
import frontMatter from "front-matter";
import { useTemplateLayoutContext } from "pages/TemplatePage/TemplateLayout";

import { pageTitle } from "utils/page";

export default function TemplateDocsPage() {
	const { template, activeVersion } = useTemplateLayoutContext();

	const readme = frontMatter(activeVersion.readme);

	return (
		<>
			<title>{pageTitle(template.name, "Documentation")}</title>

			<div
				className="rounded-lg bg-content-primary border border-solid border-zinc-700"
				id="readme"
			>
				<div className="font-semibold py-4 px-6 text-content-secondary border-0 border-b border-solid border-zinc-700">
					README.md
				</div>
				<div className="pt-0 px-6 pb-10 max-w-[800px] m-auto">
					<MemoizedMarkdown>{readme.body}</MemoizedMarkdown>
				</div>
			</div>
		</>
	);
}
