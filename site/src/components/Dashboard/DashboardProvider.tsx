import { useMachine } from "@xstate/react";
import { useBuildInfo } from "api/queries/buildInfo";
import { useEntitlements } from "api/queries/entitlements";
import {
  AppearanceConfig,
  BuildInfoResponse,
  Entitlements,
  Experiments,
} from "api/typesGenerated";
import { FullScreenLoader } from "components/Loader/FullScreenLoader";
import { createContext, FC, PropsWithChildren, useContext } from "react";
import { appearanceMachine } from "xServices/appearance/appearanceXService";
import { experimentsMachine } from "xServices/experiments/experimentsMachine";

interface Appearance {
  config: AppearanceConfig;
  preview: boolean;
  setPreview: (config: AppearanceConfig) => void;
  save: (config: AppearanceConfig) => void;
}

interface DashboardProviderValue {
  buildInfo: BuildInfoResponse;
  entitlements: Entitlements;
  appearance: Appearance;
  experiments: Experiments;
}

export const DashboardProviderContext = createContext<
  DashboardProviderValue | undefined
>(undefined);

export const DashboardProvider: FC<PropsWithChildren> = ({ children }) => {
  const buildInfo = useBuildInfo();
  const entitlements = useEntitlements();
  const [appearanceState, appearanceSend] = useMachine(appearanceMachine);
  const [experimentsState] = useMachine(experimentsMachine);
  const { appearance, preview } = appearanceState.context;
  const { experiments } = experimentsState.context;
  const isLoading =
    !buildInfo.data || !entitlements.data || !appearance || !experiments;

  const setAppearancePreview = (config: AppearanceConfig) => {
    appearanceSend({
      type: "SET_PREVIEW_APPEARANCE",
      appearance: config,
    });
  };

  const saveAppearance = (config: AppearanceConfig) => {
    appearanceSend({
      type: "SAVE_APPEARANCE",
      appearance: config,
    });
  };

  if (isLoading) {
    return <FullScreenLoader />;
  }

  return (
    <DashboardProviderContext.Provider
      value={{
        buildInfo: buildInfo.data,
        entitlements: entitlements.data,
        experiments,
        appearance: {
          preview,
          config: appearance,
          setPreview: setAppearancePreview,
          save: saveAppearance,
        },
      }}
    >
      {children}
    </DashboardProviderContext.Provider>
  );
};

export const useDashboard = (): DashboardProviderValue => {
  const context = useContext(DashboardProviderContext);

  if (!context) {
    throw new Error(
      "useDashboard only can be used inside of DashboardProvider",
    );
  }

  return context;
};

export const useIsWorkspaceActionsEnabled = (): boolean => {
  const { entitlements, experiments } = useDashboard();
  const allowAdvancedScheduling =
    entitlements.features["advanced_template_scheduling"].enabled;
  // This check can be removed when https://github.com/coder/coder/milestone/19
  // is merged up
  const allowWorkspaceActions = experiments.includes("workspace_actions");
  return allowWorkspaceActions && allowAdvancedScheduling;
};
