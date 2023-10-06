import { type FC, useEffect } from "react";
import { useLocation, useNavigate } from "react-router-dom";
import type {
  Template,
  TemplateVersion,
  WorkspaceResource,
} from "api/typesGenerated";
import { Loader } from "components/Loader/Loader";
import { Stack } from "components/Stack/Stack";
import { TemplateResourcesTable } from "components/TemplateResourcesTable/TemplateResourcesTable";
import { TemplateStats } from "./TemplateStats";
import { TemplateVersionWarnings } from "components/TemplateVersionWarnings/TemplateVersionWarnings";

export interface TemplateSummaryPageViewProps {
  resources?: WorkspaceResource[];
  template: Template;
  activeVersion: TemplateVersion;
}

export const TemplateSummaryPageView: FC<TemplateSummaryPageViewProps> = ({
  resources,
  template,
  activeVersion,
}) => {
  const navigate = useNavigate();
  const location = useLocation();

  useEffect(() => {
    if (location.hash === "#readme") {
      // We moved the readme to the docs page, but we known that some users
      // have bookmarked the readme or linked it elsewhere. Redirect them to the docs page.
      navigate(`/templates/${template.name}/docs`, { replace: true });
    }
  }, [template, navigate, location]);

  if (!resources) {
    return <Loader />;
  }

  const getStartedResources = (resources: WorkspaceResource[]) => {
    return resources.filter(
      (resource) => resource.workspace_transition === "start",
    );
  };

  return (
    <Stack spacing={4}>
      <TemplateVersionWarnings warnings={activeVersion.warnings} />
      <TemplateStats template={template} activeVersion={activeVersion} />
      <TemplateResourcesTable resources={getStartedResources(resources)} />
    </Stack>
  );
};
