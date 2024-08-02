import { createContext, type FC, type PropsWithChildren } from "react";
import { useQuery } from "react-query";
import { appearance } from "api/queries/appearance";
import { entitlements } from "api/queries/entitlements";
import { experiments } from "api/queries/experiments";
import { organizations } from "api/queries/organizations";
import type {
  AppearanceConfig,
  Entitlements,
  Experiments,
  Organization,
} from "api/typesGenerated";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { Loader } from "components/Loader/Loader";
import { useEmbeddedMetadata } from "hooks/useEmbeddedMetadata";

export interface DashboardValue {
  entitlements: Entitlements;
  experiments: Experiments;
  appearance: AppearanceConfig;
  organizations: Organization[];
}

export const DashboardContext = createContext<DashboardValue | undefined>(
  undefined,
);

export const DashboardProvider: FC<PropsWithChildren> = ({ children }) => {
  const { metadata } = useEmbeddedMetadata();
  const entitlementsQuery = useQuery(entitlements(metadata.entitlements));
  const experimentsQuery = useQuery(experiments(metadata.experiments));
  const appearanceQuery = useQuery(appearance(metadata.appearance));
  const organizationsQuery = useQuery(organizations());

  const error =
    entitlementsQuery.error ||
    appearanceQuery.error ||
    experimentsQuery.error ||
    organizationsQuery.error;

  if (error) {
    return <ErrorAlert error={error} />;
  }

  const isLoading =
    !entitlementsQuery.data ||
    !appearanceQuery.data ||
    !experimentsQuery.data ||
    !organizationsQuery.data;

  if (isLoading) {
    return <Loader fullscreen />;
  }

  return (
    <DashboardContext.Provider
      value={{
        entitlements: entitlementsQuery.data,
        experiments: experimentsQuery.data,
        appearance: appearanceQuery.data,
        organizations: organizationsQuery.data,
      }}
    >
      {children}
    </DashboardContext.Provider>
  );
};
