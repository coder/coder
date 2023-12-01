import {
  Header,
  HeaderTitle,
  Main,
  GridData,
  GridDataLabel,
  GridDataValue,
} from "./Content";

export const AccessURLPage = () => {
  return (
    <>
      <Header>
        <HeaderTitle>Access URL</HeaderTitle>
      </Header>

      <Main>
        <GridData>
          <GridDataLabel>Healthy</GridDataLabel>
          <GridDataValue>True</GridDataValue>
        </GridData>
      </Main>
    </>
  );
};

export default AccessURLPage;
