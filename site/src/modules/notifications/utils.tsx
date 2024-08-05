import EmailIcon from "@mui/icons-material/EmailOutlined";
import DeploymentIcon from "@mui/icons-material/LanguageOutlined";
import WebhookIcon from "@mui/icons-material/WebhookOutlined";

export const methodIcons: Record<string, typeof EmailIcon> = {
  "": DeploymentIcon,
  smtp: EmailIcon,
  webhook: WebhookIcon,
};

const methodLabels: Record<string, string> = {
  "": "Default",
  smtp: "SMTP",
  webhook: "Webhook",
};

export const methodLabel = (method: string, defaultMethod?: string) => {
  return method === "" && defaultMethod
    ? `${methodLabels[method]} - ${methodLabels[defaultMethod]}`
    : methodLabels[method];
};
