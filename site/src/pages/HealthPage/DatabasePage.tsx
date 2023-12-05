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
      </Header>

      <Main>
        {database.warnings.map((warning) => {
          return (
            <Alert key={warning.code} severity="warning">
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
