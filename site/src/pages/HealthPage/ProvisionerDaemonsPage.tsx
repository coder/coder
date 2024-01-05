import { Header, HeaderTitle, HealthyDot, Main, Pill } from "./Content";
import { Helmet } from "react-helmet-async";
import { pageTitle } from "utils/page";
import { useTheme } from "@mui/material/styles";
import { DismissWarningButton } from "./DismissWarningButton";
import { Alert } from "components/Alert/Alert";
import { HealthcheckReport } from "api/typesGenerated";
import { createDayString } from "utils/createDayString";

import { useOutletContext } from "react-router-dom";
import Business from "@mui/icons-material/Business";
import Person from "@mui/icons-material/Person";
import SwapHoriz from "@mui/icons-material/SwapHoriz";
import Tooltip from "@mui/material/Tooltip";
import Sell from "@mui/icons-material/Sell";

export const ProvisionerDaemonsPage = () => {
  const healthStatus = useOutletContext<HealthcheckReport>();
  const { provisioner_daemons } = healthStatus;
  const theme = useTheme();
  return (
    <>
      <Helmet>
        <title>{pageTitle("Provisioner Daemons - Health")}</title>
      </Helmet>

      <Header>
        <HeaderTitle>
          <HealthyDot severity={provisioner_daemons.severity} />
          Provisioner Daemons
        </HeaderTitle>
        <DismissWarningButton healthcheck="ProvisionerDaemons" />
      </Header>

      <Main>
        {provisioner_daemons.warnings.map((warning) => {
          return (
            <Alert key={warning.code} severity="warning">
              {warning.message}
            </Alert>
          );
        })}

        {provisioner_daemons.items.map(({ provisioner_daemon, warnings }) => {
          const daemonScope =
            provisioner_daemon.tags["scope"] || "organization";
          const iconScope =
            daemonScope === "organization" ? <Business /> : <Person />;
          const extraTags = Object.keys(provisioner_daemon.tags)
            .filter((key) => key !== "scope" && key !== "owner")
            .reduce(
              (acc, key) => {
                acc[key] = provisioner_daemon.tags[key];
                return acc;
              },
              {} as Record<string, string>,
            );
          const isWarning = warnings.length > 0;
          return (
            <div
              key={provisioner_daemon.name}
              css={{
                borderRadius: 8,
                border: `1px solid ${
                  isWarning
                    ? theme.palette.warning.light
                    : theme.palette.divider
                }`,
                fontSize: 14,
              }}
            >
              <header
                css={{
                  padding: 24,
                  display: "flex",
                  alignItems: "center",
                  justifyContenxt: "space-between",
                  gap: 24,
                }}
              >
                <div
                  css={{
                    display: "flex",
                    alignItems: "center",
                    gap: 24,
                    objectFit: "fill",
                  }}
                >
                  <div css={{ lineHeight: "160%" }}>
                    <h4 css={{ fontWeight: 500, margin: 0 }}>
                      {provisioner_daemon.name}
                    </h4>
                    <span css={{ color: theme.palette.text.secondary }}>
                      <code>{provisioner_daemon.version}</code>
                    </span>
                  </div>
                </div>
                <div
                  css={{
                    marginLeft: "auto",
                    display: "flex",
                    flexWrap: "wrap",
                    gap: 12,
                  }}
                >
                  <Tooltip title="API Version">
                    <Pill icon={<SwapHoriz />}>
                      <code>{provisioner_daemon.api_version}</code>
                    </Pill>
                  </Tooltip>
                  <Tooltip title="Scope">
                    <Pill icon={iconScope}>{titleCase(daemonScope)}</Pill>
                  </Tooltip>
                  {Object.keys(extraTags).map((k) => (
                    <Tooltip key={k} title={k}>
                      <Pill key={k} icon={<Sell />}>
                        {extraTags[k]}
                      </Pill>
                    </Tooltip>
                  ))}
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
                      <span key={i}>{warning.message}</span>
                    ))}
                  </div>
                ) : (
                  <span>No warnings</span>
                )}
                {provisioner_daemon.last_seen_at && (
                  <span
                    css={{ color: theme.palette.text.secondary }}
                    data-chromatic="ignore"
                  >
                    Last seen {createDayString(provisioner_daemon.last_seen_at)}
                  </span>
                )}
              </div>
            </div>
          );
        })}
      </Main>
    </>
  );
};

const titleCase = (s: string): string => {
  return s.charAt(0).toLocaleUpperCase() + s.slice(1);
};

export default ProvisionerDaemonsPage;
