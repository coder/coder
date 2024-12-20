import { API } from "api/api";
import { useTemplateLayoutContext } from "pages/TemplatePage/TemplateLayout";
import type { FC } from "react";
import { useQuery } from "react-query";
import { getTemplatePageTitle } from "../utils";
import { TemplateSummaryPageView } from "./TemplateSummaryPageView";

export const TemplateSummaryPage: FC = () => {
	const { template, activeVersion } = useTemplateLayoutContext();
	const { data: resources } = useQuery({
		queryKey: ["templates", template.id, "resources"],
		queryFn: () => API.getTemplateVersionResources(activeVersion.id),
	});

	return (
		<>
			<title>{getTemplatePageTitle("Template", template)}</title>

			<TemplateSummaryPageView
				resources={resources}
				template={template}
				activeVersion={activeVersion}
			/>
		</>
	);
};

export default TemplateSummaryPage;
