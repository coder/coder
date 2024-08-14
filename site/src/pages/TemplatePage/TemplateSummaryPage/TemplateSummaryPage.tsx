import type { FC } from "react";
import { Helmet } from "react-helmet-async";
import { useQuery } from "react-query";
import { API } from "api/api";
import { useTemplateLayoutContext } from "pages/TemplatePage/TemplateLayout";
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
      <Helmet>
        <title>{getTemplatePageTitle("Template", template)}</title>
      </Helmet>
      <TemplateSummaryPageView
        resources={resources}
        template={template}
        activeVersion={activeVersion}
      />
    </>
  );
};

export default TemplateSummaryPage;
