import { useOutletContext } from "react-router-dom";
import {
  Header,
  HeaderTitle,
  Main,
  GridData,
  GridDataLabel,
  GridDataValue,
  HealthyDot,
} from "./Content";
import { HealthcheckReport } from "api/typesGenerated";
import { Alert } from "components/Alert/Alert";
import { Helmet } from "react-helmet-async";
import { pageTitle } from "utils/page";
import { DismissWarningButton } from "./DismissWarningButton";

export const AccessURLPage = () => {
  const healthStatus = useOutletContext<HealthcheckReport>();
  const accessUrl = healthStatus.access_url;

  return (
    <>
      <Helmet>
        <title>{pageTitle("Access URL - Health")}</title>
      </Helmet>

      <Header>
        <HeaderTitle>
          <HealthyDot severity={accessUrl.severity} />
          Access URL
        </HeaderTitle>
        <DismissWarningButton healthcheck="AccessURL" />
      </Header>

      <Main>
        {accessUrl.warnings.map((warning) => {
          return (
            <Alert key={warning.code} severity="warning">
              {warning.message}
            </Alert>
          );
        })}

        <GridData>
          <GridDataLabel>Severity</GridDataLabel>
          <GridDataValue>{accessUrl.severity}</GridDataValue>

          <GridDataLabel>Access URL</GridDataLabel>
          <GridDataValue>{accessUrl.access_url}</GridDataValue>

          <GridDataLabel>Reachable</GridDataLabel>
          <GridDataValue>{accessUrl.reachable ? "Yes" : "No"}</GridDataValue>

          <GridDataLabel>Status Code</GridDataLabel>
          <GridDataValue>{accessUrl.status_code}</GridDataValue>
        </GridData>
      </Main>
    </>
  );
};

export default AccessURLPage;
