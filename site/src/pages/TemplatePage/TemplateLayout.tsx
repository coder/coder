import { css } from "@emotion/css";
import { useTheme } from "@emotion/react";
import { createContext, type FC, Suspense, useContext } from "react";
import { useQuery } from "react-query";
import { NavLink, Outlet, useNavigate, useParams } from "react-router-dom";
import type { AuthorizationRequest } from "api/typesGenerated";
import {
  checkAuthorization,
  getTemplateByName,
  getTemplateVersion,
} from "api/api";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { Margins } from "components/Margins/Margins";
import { Stack } from "components/Stack/Stack";
import { Loader } from "components/Loader/Loader";
import { useOrganizationId } from "hooks/useOrganizationId";
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

export const TemplateLayout: FC<{ children?: JSX.Element }> = ({
  children = <Outlet />,
}) => {
  const theme = useTheme();
  const navigate = useNavigate();
  const orgId = useOrganizationId();
  const { template: templateName } = useParams() as { template: string };
  const { data, error, isLoading } = useQuery({
    queryKey: ["template", templateName],
    queryFn: () => fetchTemplate(orgId, templateName),
  });
  const shouldShowInsights = data?.permissions?.canUpdateTemplate;

  if (error) {
    return (
      <div css={{ margin: theme.spacing(2) }}>
        <ErrorAlert error={error} />
      </div>
    );
  }

  if (isLoading || !data) {
    return <Loader />;
  }

  const itemStyles = css`
    text-decoration: none;
    color: ${theme.palette.text.secondary};
    font-size: 14;
    display: block;
    padding: ${theme.spacing(0, 2, 2)};

    &:hover {
      color: ${theme.palette.text.primary};
    }
  `;

  const activeItemStyles = css`
    ${itemStyles}
    color: ${theme.palette.text.primary};
    position: relative;

    &:before {
      content: "";
      left: 0;
      bottom: 0;
      height: 2;
      width: 100%;
      background: ${theme.palette.secondary.dark};
      position: absolute;
    }
  `;

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

      <div
        css={{
          borderBottom: `1px solid ${theme.palette.divider}`,
          marginBottom: theme.spacing(5),
        }}
      >
        <Margins>
          <Stack direction="row" spacing={0.25}>
            <NavLink
              end
              to={`/templates/${templateName}`}
              className={({ isActive }) =>
                isActive ? activeItemStyles : itemStyles
              }
            >
              Summary
            </NavLink>
            <NavLink
              end
              to={`/templates/${templateName}/docs`}
              className={({ isActive }) =>
                isActive ? activeItemStyles : itemStyles
              }
            >
              Docs
            </NavLink>
            {data.permissions.canUpdateTemplate && (
              <NavLink
                to={`/templates/${templateName}/files`}
                className={({ isActive }) =>
                  isActive ? activeItemStyles : itemStyles
                }
              >
                Source Code
              </NavLink>
            )}
            <NavLink
              to={`/templates/${templateName}/versions`}
              className={({ isActive }) =>
                isActive ? activeItemStyles : itemStyles
              }
            >
              Versions
            </NavLink>
            <NavLink
              to={`/templates/${templateName}/embed`}
              className={({ isActive }) =>
                isActive ? activeItemStyles : itemStyles
              }
            >
              Embed
            </NavLink>
            {shouldShowInsights && (
              <NavLink
                to={`/templates/${templateName}/insights`}
                className={({ isActive }) =>
                  isActive ? activeItemStyles : itemStyles
                }
              >
                Insights
              </NavLink>
            )}
          </Stack>
        </Margins>
      </div>

      <Margins>
        <TemplateLayoutContext.Provider value={data}>
          <Suspense fallback={<Loader />}>{children}</Suspense>
        </TemplateLayoutContext.Provider>
      </Margins>
    </>
  );
};
