import { useDashboard } from "modules/dashboard/useDashboard";
import { type FC, createContext } from "react";
import TemplateEmbedPage from "./TemplateEmbedPage";
import TemplateEmbedPageExperimental from "./TemplateEmbedPageExperimental";

// Similar context as in CreateWorkspaceExperimentRouter for maintaining consistency
export const ExperimentalFormContext = createContext<
  { toggleOptedOut: () => void } | undefined
>(undefined);

const TemplateEmbedExperimentRouter: FC = () => {
  const { experiments } = useDashboard();
  const dynamicParametersEnabled = experiments.includes("dynamic-parameters");

  if (dynamicParametersEnabled) {
    return <TemplateEmbedPageExperimental />;
  }

  return <TemplateEmbedPage />;
};

export default TemplateEmbedExperimentRouter;