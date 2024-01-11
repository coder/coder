import { useOutletContext } from "react-router-dom";
import {
  Header,
  HeaderTitle,
  HealthMessageDocsLink,
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

export const DatabasePage = () => {
  const healthStatus = useOutletContext<HealthcheckReport>();
  const database = healthStatus.database;

  return (
    <>
      <Helmet>
        <title>{pageTitle("Database - Health")}</title>
      </Helmet>

      <Header>
        <HeaderTitle>
          <HealthyDot severity={database.severity} />
          Database
        </HeaderTitle>
        <DismissWarningButton healthcheck="Database" />
      </Header>

      <Main>
        {database.warnings.map((warning) => {
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

        <GridData>
          <GridDataLabel>Reachable</GridDataLabel>
          <GridDataValue>{database.reachable ? "Yes" : "No"}</GridDataValue>

          <GridDataLabel>Latency</GridDataLabel>
          <GridDataValue>{database.latency_ms}ms</GridDataValue>

          <GridDataLabel>Threshold</GridDataLabel>
          <GridDataValue>{database.threshold_ms}ms</GridDataValue>
        </GridData>
      </Main>
    </>
  );
};

export default DatabasePage;
