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

export const AccessURLPage = () => {
  const healthStatus = useOutletContext<HealthcheckReport>();
  const accessUrl = healthStatus.access_url;

  return (
    <>
      <Header>
        <HeaderTitle>Access URL</HeaderTitle>
      </Header>

      <Main>
        <GridData>
          <GridDataLabel>Healthy</GridDataLabel>
          <GridDataValue>{accessUrl.healthy ? "Yes" : "No"}</GridDataValue>

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
