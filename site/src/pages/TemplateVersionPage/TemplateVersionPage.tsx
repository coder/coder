import { useMachine } from "@xstate/react";
import { usePermissions } from "hooks/usePermissions";
import { useOrganizationId } from "hooks/useOrganizationId";
import { useTab } from "hooks/useTab";
import { type FC, useMemo } from "react";
import { Helmet } from "react-helmet-async";
import { useParams } from "react-router-dom";
import { pageTitle } from "utils/page";
import { templateVersionMachine } from "xServices/templateVersion/templateVersionXService";
import TemplateVersionPageView from "./TemplateVersionPageView";

type Params = {
  version: string;
  template: string;
};

export const TemplateVersionPage: FC = () => {
  const { version: versionName, template: templateName } =
    useParams() as Params;
  const orgId = useOrganizationId();
  const [state] = useMachine(templateVersionMachine, {
    context: { templateName, versionName, orgId },
  });
  const tab = useTab("file", "0");
  const permissions = usePermissions();

  const versionId = state.context.currentVersion?.id;
  const createWorkspaceUrl = useMemo(() => {
    const params = new URLSearchParams();
    if (versionId) {
      params.set("version", versionId);
      return `/templates/${templateName}/workspace?${params.toString()}`;
    }
    return undefined;
  }, [templateName, versionId]);

  return (
    <>
      <Helmet>
        <title>{pageTitle(`Version ${versionName} · ${templateName}`)}</title>
      </Helmet>

      <TemplateVersionPageView
        context={state.context}
        versionName={versionName}
        templateName={templateName}
        tab={tab}
        createWorkspaceUrl={
          permissions.updateTemplates ? createWorkspaceUrl : undefined
        }
      />
    </>
  );
};

export default TemplateVersionPage;
