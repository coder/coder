import {
  createContext,
  type FC,
  type PropsWithChildren,
  Suspense,
  useContext,
} from "react";
import { useQuery } from "react-query";
import { Outlet, useNavigate, useParams } from "react-router-dom";
import type { AuthorizationRequest } from "api/typesGenerated";
import {
  checkAuthorization,
  getTemplateByName,
  getTemplateVersion,
} from "api/api";
import { useOrganizationId } from "contexts/auth/useOrganizationId";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { Margins } from "components/Margins/Margins";
import { Loader } from "components/Loader/Loader";
import { TabLink, Tabs } from "components/Tabs/Tabs";
import { TemplatePageHeader } from "./TemplatePageHeader";

const templatePermissions = (
  templateId: string,
): AuthorizationRequest["checks"] => ({
  canUpdateTemplate: {
    object: {
      resource_type: "template",
      resource_id: templateId,
    },
    action: "update",
  },
  canReadInsights: {
    object: {
      resource_type: "template_insights",
    },
    action: "read",
  },
});

const fetchTemplate = async (orgId: string, templateName: string) => {
  const template = await getTemplateByName(orgId, templateName);
  const [activeVersion, permissions] = await Promise.all([
    getTemplateVersion(template.active_version_id),
    checkAuthorization({
      checks: templatePermissions(template.id),
    }),
  ]);

  return {
    template,
    activeVersion,
    permissions,
  };
};

type TemplateLayoutContextValue = Awaited<ReturnType<typeof fetchTemplate>>;

const TemplateLayoutContext = createContext<
  TemplateLayoutContextValue | undefined
>(undefined);

export const useTemplateLayoutContext = (): TemplateLayoutContextValue => {
  const context = useContext(TemplateLayoutContext);
  if (!context) {
    throw new Error(
      "useTemplateLayoutContext only can be used inside of TemplateLayout",
    );
  }
  return context;
};

export const TemplateLayout: FC<PropsWithChildren> = ({
  children = <Outlet />,
}) => {
  const navigate = useNavigate();
  const orgId = useOrganizationId();
  const { template: templateName } = useParams() as { template: string };
  const { data, error, isLoading } = useQuery({
    queryKey: ["template", templateName],
    queryFn: () => fetchTemplate(orgId, templateName),
  });
  // Auditors should also be able to view insights, but do not automatically
  // have permission to update templates. Need both checks.
  const shouldShowInsights =
    data?.permissions?.canUpdateTemplate || data?.permissions?.canReadInsights;

  if (error) {
    return (
      <div css={{ margin: 16 }}>
        <ErrorAlert error={error} />
      </div>
    );
  }

  if (isLoading || !data) {
    return <Loader />;
  }

  return (
    <>
      <TemplatePageHeader
        template={data.template}
        activeVersion={data.activeVersion}
        permissions={data.permissions}
        onDeleteTemplate={() => {
          navigate("/templates");
        }}
      />

      <Tabs>
        <TabLink end to={`/templates/${templateName}`}>
          Summary
        </TabLink>
        <TabLink to={`/templates/${templateName}/docs`}>Docs</TabLink>
        {data.permissions.canUpdateTemplate && (
          <TabLink to={`/templates/${templateName}/files`}>Source Code</TabLink>
        )}
        <TabLink to={`/templates/${templateName}/versions`}>Versions</TabLink>
        <TabLink to={`/templates/${templateName}/embed`}>Embed</TabLink>
        {shouldShowInsights && (
          <TabLink to={`/templates/${templateName}/insights`}>Insights</TabLink>
        )}
      </Tabs>

      <Margins>
        <TemplateLayoutContext.Provider value={data}>
          <Suspense fallback={<Loader />}>{children}</Suspense>
        </TemplateLayoutContext.Provider>
      </Margins>
    </>
  );
};
