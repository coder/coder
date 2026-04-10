import frontMatter from "front-matter";
import { MemoizedMarkdown } from "#/components/Markdown/Markdown";
import { useTemplateLayoutContext } from "#/pages/TemplatePage/TemplateLayout";
import { pageTitle } from "#/utils/page";

export default function TemplateDocsPage() {
	const { template, activeVersion } = useTemplateLayoutContext();
	const readme = frontMatter(activeVersion.readme);

	return (
		<>
			<title>{pageTitle(template.name, "Documentation")}</title>
			<div
				className="bg-surface-primary border border-solid border-border rounded-lg"
				id="readme"
			>
				<div className="text-content-secondary font-semibold py-4 px-6 border-b border-border">
					README.md
				</div>
				<div className="px-6 pb-10 max-w-[800px] mx-auto">
					<MemoizedMarkdown>{readme.body}</MemoizedMarkdown>
				</div>
			</div>
		</>
	);
}
