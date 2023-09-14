import { useQuery } from "@tanstack/react-query";
import { buildInfo } from "api/queries/buildInfo";
import { experiments } from "api/queries/experiments";
import { entitlements } from "api/queries/entitlements";
import {
  AppearanceConfig,
  BuildInfoResponse,
  Entitlements,
  Experiments,
} from "api/typesGenerated";
import { FullScreenLoader } from "components/Loader/FullScreenLoader";
import {
  createContext,
  FC,
  PropsWithChildren,
  useContext,
  useState,
} from "react";
import { appearance } from "api/queries/appearance";

interface Appearance {
  config: AppearanceConfig;
  isPreview: boolean;
  setPreview: (config: AppearanceConfig) => void;
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
  const buildInfoQuery = useQuery(buildInfo());
  const entitlementsQuery = useQuery(entitlements());
  const experimentsQuery = useQuery(experiments());
  const appearanceQuery = useQuery(appearance());
  const isLoading =
    !buildInfoQuery.data ||
    !entitlementsQuery.data ||
    !appearanceQuery.data ||
    !experimentsQuery.data;
  const [configPreview, setConfigPreview] = useState<AppearanceConfig>();

  if (isLoading) {
    return <FullScreenLoader />;
  }

  return (
    <DashboardProviderContext.Provider
      value={{
        buildInfo: buildInfoQuery.data,
        entitlements: entitlementsQuery.data,
        experiments: experimentsQuery.data,
        appearance: {
          config: configPreview ?? appearanceQuery.data,
          setPreview: setConfigPreview,
          isPreview: configPreview !== undefined,
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
