import { Link, useOutletContext, useParams } from "react-router-dom";
import {
  Header,
  HeaderTitle,
  Main,
  BooleanPill,
  Pill,
  Logs,
  HealthyDot,
} from "./Content";
import {
  HealthMessage,
  HealthSeverity,
  HealthcheckReport,
} from "api/typesGenerated";
import CodeOutlined from "@mui/icons-material/CodeOutlined";
import TagOutlined from "@mui/icons-material/TagOutlined";
import Tooltip from "@mui/material/Tooltip";
import { useTheme } from "@mui/material/styles";
import ArrowBackOutlined from "@mui/icons-material/ArrowBackOutlined";
import { getLatencyColor } from "utils/latency";
import { Alert } from "components/Alert/Alert";
import { Helmet } from "react-helmet-async";
import { pageTitle } from "utils/page";

export const DERPRegionPage = () => {
  const theme = useTheme();
  const healthStatus = useOutletContext<HealthcheckReport>();
  const params = useParams() as { regionId: string };
  const regionId = Number(params.regionId);
  const {
    region,
    node_reports: reports,
    warnings,
    severity,
  } = healthStatus.derp.regions[regionId];

  return (
    <>
      <Helmet>
        <title>{pageTitle(`${region.RegionName} - Health`)}</title>
      </Helmet>

      <Header>
        <hgroup>
          <Link
            css={{
              fontSize: 12,
              textDecoration: "none",
              color: theme.palette.text.secondary,
              fontWeight: 500,
              display: "inline-flex",
              alignItems: "center",
              "&:hover": {
                color: theme.palette.text.primary,
              },
              marginBottom: 8,
              lineHeight: "120%",
            }}
            to="/health/derp"
          >
            <ArrowBackOutlined
              css={{ fontSize: 12, verticalAlign: "middle", marginRight: 8 }}
            />
            Back to DERP
          </Link>
          <HeaderTitle>
            <HealthyDot severity={severity as HealthSeverity} />
            {region.RegionName}
          </HeaderTitle>
        </hgroup>
      </Header>

      <Main>
        {warnings.map((warning: HealthMessage) => {
          return (
            <Alert key={warning.code} severity="warning">
              {warning.message}
            </Alert>
          );
        })}

        <section>
          <div css={{ display: "flex", flexWrap: "wrap", gap: 12 }}>
            <Tooltip title="Region ID">
              <Pill icon={<TagOutlined />}>{region.RegionID}</Pill>
            </Tooltip>
            <Tooltip title="Region Code">
              <Pill icon={<CodeOutlined />}>{region.RegionCode}</Pill>
            </Tooltip>
            <BooleanPill value={region.EmbeddedRelay}>
              Embedded Relay
            </BooleanPill>
          </div>
        </section>

        {reports.map((report) => {
          const { node, client_logs: logs } = report;
          const latencyColor = getLatencyColor(
            theme,
            report.round_trip_ping_ms,
          );
          return (
            <section
              key={node.HostName}
              css={{
                border: `1px solid ${theme.palette.divider}`,
                borderRadius: 8,
                fontSize: 14,
              }}
            >
              <header
                css={{
                  padding: 24,
                  display: "flex",
                  justifyContent: "space-between",
                  alignItems: "center",
                }}
              >
                <div>
                  <h4
                    css={{
                      fontWeight: 500,
                      margin: 0,
                      lineHeight: "120%",
                    }}
                  >
                    {node.HostName}
                  </h4>
                  <div
                    css={{
                      display: "flex",
                      alignItems: "center",
                      gap: 8,
                      color: theme.palette.text.secondary,
                      fontSize: 12,
                      lineHeight: "120%",
                      marginTop: 8,
                    }}
                  >
                    <span>DERP Port: {node.DERPPort ?? "None"}</span>
                    <span>STUN Port: {node.STUNPort ?? "None"}</span>
                  </div>
                </div>

                <div css={{ display: "flex", gap: 8, alignItems: "center" }}>
                  <Tooltip title="Round trip ping">
                    <Pill
                      css={{ color: latencyColor }}
                      icon={
                        <div
                          css={{
                            display: "flex",
                            alignItems: "center",
                            justifyContent: "center",
                          }}
                        >
                          <div
                            css={{
                              width: 8,
                              height: 8,
                              backgroundColor: latencyColor,
                              borderRadius: 9999,
                            }}
                          />
                        </div>
                      }
                    >
                      {report.round_trip_ping_ms}ms
                    </Pill>
                  </Tooltip>
                  <BooleanPill value={report.can_exchange_messages}>
                    Exchange Messages
                  </BooleanPill>
                  <BooleanPill value={report.uses_websocket}>
                    Websocket
                  </BooleanPill>
                </div>
              </header>
              <Logs
                lines={logs?.[0] ?? []}
                css={{
                  borderBottomLeftRadius: 8,
                  borderBottomRightRadius: 8,
                  borderTop: `1px solid ${theme.palette.divider}`,
                }}
              />
            </section>
          );
        })}
      </Main>
    </>
  );
};

export default DERPRegionPage;
