import Typography from "@mui/material/Typography";
import { type FC } from "react";

export const NotFoundPage: FC = () => {
  return (
    <div
      css={{
        width: "100%",
        height: "100%",
        display: "flex",
        flexDirection: "row",
        justifyContent: "center",
        alignItems: "center",
      }}
    >
      <div
        css={(theme) => ({
          margin: 8,
          padding: 8,
          borderRight: theme.palette.divider,
        })}
      >
        <Typography variant="h4">404</Typography>
      </div>
      <Typography variant="body2">This page could not be found.</Typography>
    </div>
  );
};

export default NotFoundPage;
