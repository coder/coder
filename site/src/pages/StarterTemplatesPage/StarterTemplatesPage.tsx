import { useMachine } from "@xstate/react";
import { useOrganizationId } from "hooks/useOrganizationId";
import { FC } from "react";
import { Helmet } from "react-helmet-async";
import { useTranslation } from "react-i18next";
import { pageTitle } from "utils/page";
import { starterTemplatesMachine } from "xServices/starterTemplates/starterTemplatesXService";
import { StarterTemplatesPageView } from "./StarterTemplatesPageView";

const StarterTemplatesPage: FC = () => {
  const { t } = useTranslation("starterTemplatesPage");
  const organizationId = useOrganizationId();
  const [state] = useMachine(starterTemplatesMachine, {
    context: { organizationId },
  });

  return (
    <>
      <Helmet>
        <title>{pageTitle(t("title").toString())}</title>
      </Helmet>

      <StarterTemplatesPageView context={state.context} />
    </>
  );
};

export default StarterTemplatesPage;
