import { useTemplateLayoutContext } from "components/TemplateLayout/TemplateLayout";
import { FC } from "react";
import { Helmet } from "react-helmet-async";
import { getTemplatePageTitle } from "../utils";
import { TemplateSummaryPageView } from "./TemplateSummaryPageView";
import { useQuery } from "@tanstack/react-query";
import { getTemplateVersionResources } from "api/api";

export const TemplateSummaryPage: FC = () => {
  const { template, activeVersion } = useTemplateLayoutContext();
  const { data: resources } = useQuery({
    queryKey: ["templates", template.id, "resources"],
    queryFn: () => getTemplateVersionResources(activeVersion.id),
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
