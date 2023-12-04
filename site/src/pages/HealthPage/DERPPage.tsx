import { Link, useOutletContext } from "react-router-dom";
import {
  Header,
  HeaderTitle,
  Main,
  SectionLabel,
  BooleanPill,
  Logs,
  HealthyDot,
} from "./Content";
import { HealthcheckReport } from "api/typesGenerated";
import Button from "@mui/material/Button";
import LocationOnOutlined from "@mui/icons-material/LocationOnOutlined";
import useTheme from "@mui/styles/useTheme";
import { healthyColor } from "./healthyColor";
import { Alert } from "components/Alert/Alert";

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
      <Header>
        <HeaderTitle>
          <HealthyDot
            healthy={derp.healthy}
            hasWarnings={derp.warnings.length > 0}
          />
          DERP
        </HeaderTitle>
      </Header>

      <Main>
        {derp.warnings.map((warning, i) => {
          return (
            <Alert key={i} severity="warning">
              {warning}
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
            {Object.values(regions).map(({ region, healthy, warnings }) => {
              return (
                <Button
                  startIcon={
                    <LocationOnOutlined
                      css={{
                        width: 16,
                        height: 16,
                        color: healthyColor(
                          theme,
                          healthy,
                          warnings?.length > 0,
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
