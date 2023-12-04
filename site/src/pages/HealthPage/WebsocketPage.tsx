import { useOutletContext } from "react-router-dom";
import { Header, HeaderTitle, Main, Pill, SectionLabel } from "./Content";
import { HealthcheckReport } from "api/typesGenerated";
import CodeOutlined from "@mui/icons-material/CodeOutlined";
import Tooltip from "@mui/material/Tooltip";
import { useTheme } from "@mui/material/styles";
import { MONOSPACE_FONT_FAMILY } from "theme/constants";
import { Alert } from "components/Alert/Alert";

export const WebsocketPage = () => {
  const healthStatus = useOutletContext<HealthcheckReport>();
  const { websocket } = healthStatus;
  const theme = useTheme();

  return (
    <>
      <Header>
        <HeaderTitle>Websocket</HeaderTitle>
      </Header>

      <Main>
        {websocket.warnings.map((warning, i) => {
          return (
            <Alert key={i} severity="warning">
              {warning}
            </Alert>
          );
        })}

        <section>
          <Tooltip title="Code">
            <Pill icon={<CodeOutlined />}>{websocket.code}</Pill>
          </Tooltip>
        </section>

        <section>
          <SectionLabel>Body</SectionLabel>
          <div
            css={{
              backgroundColor: theme.palette.background.paper,
              border: `1px solid ${theme.palette.divider}`,
              borderRadius: 8,
              fontSize: 14,
              padding: 24,
              fontFamily: MONOSPACE_FONT_FAMILY,
            }}
          >
            {websocket.body !== "" ? (
              websocket.body
            ) : (
              <span css={{ color: theme.palette.text.secondary }}>
                No body message
              </span>
            )}
          </div>
        </section>
      </Main>
    </>
  );
};

export default WebsocketPage;
