import { GridData, GridDataLabel, GridDataValue } from "./GridData";
import { Header, HeaderTitle } from "./Header";

export const AccessURLPage = () => {
  return (
    <>
      <Header>
        <HeaderTitle>Access URL</HeaderTitle>
      </Header>
      <main css={{ padding: "0 36px" }}>
        <GridData>
          <GridDataLabel>Healthy</GridDataLabel>
          <GridDataValue>True</GridDataValue>
        </GridData>
      </main>
    </>
  );
};

export default AccessURLPage;
