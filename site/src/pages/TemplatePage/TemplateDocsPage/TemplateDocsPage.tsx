import { useQuery } from "react-query";
import { templateVersionDocs } from "#/api/queries/templates";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Loader } from "#/components/Loader/Loader";
import { useTemplateLayoutContext } from "#/pages/TemplatePage/TemplateLayout";
import { pageTitle } from "#/utils/page";
import { TemplateDocsPageView } from "./TemplateDocsPageView";

export default function TemplateDocsPage() {
	const { template, activeVersion } = useTemplateLayoutContext();
	const { data, error, isLoading } = useQuery(
		templateVersionDocs(activeVersion.id),
	);
	return (
		<>
			<title>{pageTitle(template.name, "Documentation")}</title>
			{error && <ErrorAlert error={error} />}
			{isLoading ? (
				<Loader />
			) : (
				<TemplateDocsPageView
					files={data?.files ?? { "README.md": activeVersion.readme }}
				/>
			)}
		</>
	);
}
