import { useOutletContext } from "react-router-dom";
import {
  BooleanPill,
  Header,
  HeaderTitle,
  HealthyDot,
  Main,
  Pill,
} from "./Content";
import { HealthcheckReport } from "api/typesGenerated";
import { useTheme } from "@mui/material/styles";
import { createDayString } from "utils/createDayString";
import PublicOutlined from "@mui/icons-material/PublicOutlined";
import Tooltip from "@mui/material/Tooltip";
import TagOutlined from "@mui/icons-material/TagOutlined";
import { Alert } from "components/Alert/Alert";
import { Helmet } from "react-helmet-async";
import { pageTitle } from "utils/page";

export const WorkspaceProxyPage = () => {
  const healthStatus = useOutletContext<HealthcheckReport>();
  const { workspace_proxy } = healthStatus;
  const { regions } = workspace_proxy.workspace_proxies;
  const theme = useTheme();

  return (
    <>
      <Helmet>
        <title>{pageTitle("Workspace Proxy - Health")}</title>
      </Helmet>

      <Header>
        <HeaderTitle>
          <HealthyDot severity={workspace_proxy.severity} />
          Workspace Proxy
        </HeaderTitle>
      </Header>

      <Main>
        {workspace_proxy.warnings.map((warning) => {
          return (
            <Alert key={warning.code} severity="warning">
              {warning.message}
            </Alert>
          );
        })}

        {regions.map((region) => {
          const warnings = region.status?.report?.warnings ?? [];

          return (
            <div
              key={region.id}
              css={{
                borderRadius: 8,
                border: `1px solid ${theme.palette.divider}`,
                fontSize: 14,
              }}
            >
              <header
                css={{
                  padding: 24,
                  display: "flex",
                  alignItems: "center",
                  justifyContent: "space-between",
                  gap: 24,
                }}
              >
                <div css={{ display: "flex", alignItems: "center", gap: 24 }}>
                  <div
                    css={{
                      width: 36,
                      height: 36,
                      display: "flex",
                      alignItems: "center",
                      justifyContent: "center",
                    }}
                  >
                    <img
                      src={region.icon_url}
                      css={{ objectFit: "fill", width: "100%", height: "100%" }}
                      alt=""
                    />
                  </div>
                  <div css={{ lineHeight: "160%" }}>
                    <h4 css={{ fontWeight: 500, margin: 0 }}>
                      {region.display_name}
                    </h4>
                    <span css={{ color: theme.palette.text.secondary }}>
                      {region.version}
                    </span>
                  </div>
                </div>

                <div css={{ display: "flex", flexWrap: "wrap", gap: 12 }}>
                  {region.wildcard_hostname && (
                    <Tooltip title="Wildcard Hostname">
                      <Pill icon={<PublicOutlined />}>
                        {region.wildcard_hostname}
                      </Pill>
                    </Tooltip>
                  )}
                  {region.version && (
                    <Tooltip title="Version">
                      <Pill icon={<TagOutlined />}>{region.version}</Pill>
                    </Tooltip>
                  )}
                  <BooleanPill value={region.derp_enabled}>
                    DERP Enabled
                  </BooleanPill>
                  <BooleanPill value={region.derp_only}>DERP Only</BooleanPill>
                  <BooleanPill value={region.deleted}>Deleted</BooleanPill>
                </div>
              </header>

              <div
                css={{
                  borderTop: `1px solid ${theme.palette.divider}`,
                  display: "flex",
                  alignItems: "center",
                  justifyContent: "space-between",
                  padding: "8px 24px",
                  fontSize: 12,
                  color: theme.palette.text.secondary,
                }}
              >
                {warnings.length > 0 ? (
                  <div css={{ display: "flex", flexDirection: "column" }}>
                    {warnings.map((warning, i) => (
                      <span key={i}>{warning}</span>
                    ))}
                  </div>
                ) : (
                  <span>No warnings</span>
                )}
                <span data-chromatic="ignore">
                  {createDayString(region.updated_at)}
                </span>
              </div>
            </div>
          );
        })}
      </Main>
    </>
  );
};

export default WorkspaceProxyPage;
