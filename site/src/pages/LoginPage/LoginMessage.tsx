import { Alert } from "components/Alert/Alert";
import { type FC } from "react";
import { useLocation } from "react-router-dom";

export const LoginMessage: FC = () => {
  const location = useLocation();
  const params = new URLSearchParams(location.search);
  const message = params.get("message");
  const isHtml = params.get("html") === "true";

  if (!message) {
    return null;
  }

  return (
    <Alert severity="info">
      {isHtml ? (
        <span
          dangerouslySetInnerHTML={{ __html: decodeURIComponent(message) }}
          css={{
            "& .link": {
              color: "inherit",
              textDecoration: "underline",
              "&:hover": {
                textDecoration: "none",
              },
            },
          }}
        />
      ) : (
        message
      )}
    </Alert>
  );
}; 
