import { useMachine } from "@xstate/react";
import { useOrganizationId } from "hooks/useOrganizationId";
import { FC } from "react";
import { Helmet } from "react-helmet-async";
import { useParams } from "react-router-dom";
import { pageTitle } from "utils/page";
import { starterTemplateMachine } from "xServices/starterTemplates/starterTemplateXService";
import { StarterTemplatePageView } from "./StarterTemplatePageView";

const StarterTemplatePage: FC = () => {
  const { exampleId } = useParams() as { exampleId: string };
  const organizationId = useOrganizationId();
  const [state] = useMachine(starterTemplateMachine, {
    context: {
      organizationId,
      exampleId,
    },
  });

  return (
    <>
      <Helmet>
        <title>
          {pageTitle(state.context.starterTemplate?.name ?? exampleId)}
        </title>
      </Helmet>

      <StarterTemplatePageView context={state.context} />
    </>
  );
};

export default StarterTemplatePage;
