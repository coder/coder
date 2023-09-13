import { useMachine } from "@xstate/react";
import { useOrganizationId } from "hooks/useOrganizationId";
import { useTab } from "hooks/useTab";
import { FC } from "react";
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
      />
    </>
  );
};

export default TemplateVersionPage;
