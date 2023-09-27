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
  type FC,
  type PropsWithChildren,
  createContext,
  useCallback,
  useContext,
  useState,
} from "react";
import { appearance } from "api/queries/appearance";
import { hslToHex, isHexColor, isHslColor } from "utils/colors";
import { displayError } from "components/GlobalSnackbar/utils";

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

  // Centralizing the logic for catching malformed configs in one spot, just to
  // be on the safe side; don't want to expose raw setConfigPreview outside
  // the provider
  const setPreview = useCallback((newConfig: AppearanceConfig) => {
    // Have runtime safety nets in place, just because so much of the codebase
    // relies on HSL for formatting, but server expects hex values. Can't catch
    // color format mismatches at the type level
    const incomingBg = newConfig.service_banner.background_color;
    let configForDispatch = newConfig;

    if (typeof incomingBg === "string" && isHslColor(incomingBg)) {
      configForDispatch = {
        ...newConfig,
        service_banner: {
          ...newConfig.service_banner,
          background_color: hslToHex(incomingBg),
        },
      };
    } else if (typeof incomingBg === "string" && !isHexColor(incomingBg)) {
      displayError(`The value ${incomingBg} is not a valid hex string`);
      return;
    }

    setConfigPreview(configForDispatch);
  }, []);

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
          setPreview: setPreview,
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
