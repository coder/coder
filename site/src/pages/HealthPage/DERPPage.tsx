import { Link, useOutletContext } from "react-router-dom";
import {
  Header,
  HeaderTitle,
  HealthMessageDocsLink,
  Main,
  SectionLabel,
  BooleanPill,
  Logs,
  HealthyDot,
} from "./Content";
import {
  HealthMessage,
  HealthSeverity,
  HealthcheckReport,
} from "api/typesGenerated";
import Button from "@mui/material/Button";
import LocationOnOutlined from "@mui/icons-material/LocationOnOutlined";
import { healthyColor } from "./healthyColor";
import { Alert } from "components/Alert/Alert";
import { Helmet } from "react-helmet-async";
import { pageTitle } from "utils/page";
import { useTheme } from "@mui/material/styles";
import { DismissWarningButton } from "./DismissWarningButton";

const flags = [
  "UDP",
  "IPv6",
  "IPv4",
  "IPv6CanSend",
  "IPv4CanSend",
  "OSHasIPv6",
  "ICMPv4",
  "MappingVariesByDestIP",
  "HairPinning",
  "UPnP",
  "PMP",
  "PCP",
];

export const DERPPage = () => {
  const { derp } = useOutletContext<HealthcheckReport>();
  const { netcheck, regions, netcheck_logs: logs } = derp;
  const theme = useTheme();

  return (
    <>
      <Helmet>
        <title>{pageTitle("DERP - Health")}</title>
      </Helmet>

      <Header>
        <HeaderTitle>
          <HealthyDot severity={derp.severity as HealthSeverity} />
          DERP
        </HeaderTitle>
        <DismissWarningButton healthcheck="DERP" />
      </Header>

      <Main>
        {derp.warnings.map((warning: HealthMessage) => {
          return (
            <Alert
              actions={HealthMessageDocsLink(warning)}
              key={warning.code}
              severity="warning"
            >
              {warning.message}
            </Alert>
          );
        })}

        <section>
          <SectionLabel>Flags</SectionLabel>
          <div css={{ display: "flex", flexWrap: "wrap", gap: 12 }}>
            {flags.map((flag) => (
              <BooleanPill key={flag} value={netcheck[flag]}>
                {flag}
              </BooleanPill>
            ))}
          </div>
        </section>

        <section>
          <SectionLabel>Regions</SectionLabel>
          <div css={{ display: "flex", flexWrap: "wrap", gap: 12 }}>
            {Object.values(regions)
              .sort((a, b) => {
                if (a.region && b.region) {
                  return a.region.RegionName.localeCompare(b.region.RegionName);
                }
              })
              .map(({ severity, region }) => {
                return (
                  <Button
                    startIcon={
                      <LocationOnOutlined
                        css={{
                          width: 16,
                          height: 16,
                          color: healthyColor(
                            theme,
                            severity as HealthSeverity,
                          ),
                        }}
                      />
                    }
                    component={Link}
                    to={`/health/derp/regions/${region.RegionID}`}
                    key={region.RegionID}
                  >
                    {region.RegionName}
                  </Button>
                );
              })}
          </div>
        </section>

        <section>
          <SectionLabel>Logs</SectionLabel>
          <Logs
            lines={logs}
            css={(theme) => ({
              borderRadius: 8,
              border: `1px solid ${theme.palette.divider}`,
              color: theme.palette.text.secondary,
            })}
          />
        </section>
      </Main>
    </>
  );
};

export default DERPPage;
