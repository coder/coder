import {
  createContext,
  type FC,
  type PropsWithChildren,
  useState,
} from "react";
import { useQuery } from "react-query";
import { appearance } from "api/queries/appearance";
import { entitlements } from "api/queries/entitlements";
import { experiments } from "api/queries/experiments";
import type {
  AppearanceConfig,
  Entitlements,
  Experiments,
} from "api/typesGenerated";
import { Loader } from "components/Loader/Loader";
import { useAuthenticated } from "contexts/auth/RequireAuth";
import { useEffectEvent } from "hooks/hookPolyfills";
import { useEmbeddedMetadata } from "hooks/useEmbeddedMetadata";

export interface DashboardValue {
  organizationId: string;
  setOrganizationId: (id: string) => void;
  entitlements: Entitlements;
  experiments: Experiments;
  appearance: AppearanceConfig;
}

export const DashboardContext = createContext<DashboardValue | undefined>(
  undefined,
);

export const DashboardProvider: FC<PropsWithChildren> = ({ children }) => {
  const { metadata } = useEmbeddedMetadata();
  const { user, organizationIds } = useAuthenticated();
  const entitlementsQuery = useQuery(entitlements(metadata.entitlements));
  const experimentsQuery = useQuery(experiments(metadata.experiments));
  const appearanceQuery = useQuery(appearance(metadata.appearance));

  const isLoading =
    !entitlementsQuery.data || !appearanceQuery.data || !experimentsQuery.data;

  const lastUsedOrganizationId = localStorage.getItem(
    `user:${user.id}.lastUsedOrganizationId`,
  );
  const [activeOrganizationId, setActiveOrganizationId] = useState(() =>
    lastUsedOrganizationId && organizationIds.includes(lastUsedOrganizationId)
      ? lastUsedOrganizationId
      : organizationIds[0],
  );

  const setOrganizationId = useEffectEvent((id: string) => {
    if (!organizationIds.includes(id)) {
      throw new ReferenceError("Invalid organization ID");
    }
    localStorage.setItem(`user:${user.id}.lastUsedOrganizationId`, id);
    setActiveOrganizationId(id);
  });

  if (isLoading) {
    return <Loader fullscreen />;
  }

  return (
    <DashboardContext.Provider
      value={{
        organizationId: activeOrganizationId,
        setOrganizationId: setOrganizationId,
        entitlements: entitlementsQuery.data,
        experiments: experimentsQuery.data,
        appearance: appearanceQuery.data,
      }}
    >
      {children}
    </DashboardContext.Provider>
  );
};
