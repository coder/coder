import { API } from "api/api";
import { useTemplateLayoutContext } from "pages/TemplatePage/TemplateLayout";
import type { FC } from "react";
import { useQuery } from "react-query";
import { getTemplatePageTitle } from "../utils";
import { TemplateResourcesPageView } from "./TemplateResourcesPageView";

const TemplateResourcesPage: FC = () => {
	const { template, activeVersion } = useTemplateLayoutContext();
	const { data: resources } = useQuery({
		queryKey: ["templates", template.id, "resources"],
		queryFn: () => API.getTemplateVersionResources(activeVersion.id),
	});

	return (
		<>
			<title>{getTemplatePageTitle("Template", template)}</title>

			<TemplateResourcesPageView resources={resources} template={template} />
		</>
	);
};

export default TemplateResourcesPage;
