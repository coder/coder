import { useOutletContext } from "react-router-dom";
import {
  Header,
  HeaderTitle,
  Main,
  GridData,
  GridDataLabel,
  GridDataValue,
} from "./Content";
import { HealthcheckReport } from "api/typesGenerated";
import { Alert } from "components/Alert/Alert";

export const DatabasePage = () => {
  const healthStatus = useOutletContext<HealthcheckReport>();
  const database = healthStatus.database;

  return (
    <>
      <Header>
        <HeaderTitle>Database</HeaderTitle>
      </Header>

      <Main>
        {database.warnings.map((warning, i) => {
          return (
            <Alert key={i} severity="warning">
              {warning.message}
            </Alert>
          );
        })}

        <GridData>
          <GridDataLabel>Healthy</GridDataLabel>
          <GridDataValue>{database.healthy ? "Yes" : "No"}</GridDataValue>

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
