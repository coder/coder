import { useMachine } from "@xstate/react";
import { useOrganizationId } from "hooks/useOrganizationId";
import { usePermissions } from "hooks/usePermissions";
import { FC } from "react";
import { Helmet } from "react-helmet-async";
import { pageTitle } from "../../utils/page";
import { templatesMachine } from "../../xServices/templates/templatesXService";
import { TemplatesPageView } from "./TemplatesPageView";

export const TemplatesPage: FC = () => {
  const organizationId = useOrganizationId();
  const permissions = usePermissions();
  const [templatesState] = useMachine(templatesMachine, {
    context: {
      organizationId,
      permissions,
    },
  });

  return (
    <>
      <Helmet>
        <title>{pageTitle("Templates")}</title>
      </Helmet>
      <TemplatesPageView context={templatesState.context} />
    </>
  );
};

export default TemplatesPage;
