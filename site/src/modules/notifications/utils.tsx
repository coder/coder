import EmailIcon from "@mui/icons-material/EmailOutlined";
import DeploymentIcon from "@mui/icons-material/LanguageOutlined";
import WebhookIcon from "@mui/icons-material/WebhookOutlined";
import type { NotificationTemplateMethod } from "api/typesGenerated";

export const methodIcons: Record<NotificationTemplateMethod, typeof EmailIcon> =
  {
    "": DeploymentIcon,
    smtp: EmailIcon,
    webhook: WebhookIcon,
  };

const methodLabels: Record<NotificationTemplateMethod, string> = {
  "": "Default",
  smtp: "SMTP",
  webhook: "Webhook",
};

export const methodLabel = (
  method: NotificationTemplateMethod,
  defaultMethod?: NotificationTemplateMethod,
) => {
  return method === "" && defaultMethod
    ? `${methodLabels[method]} - ${methodLabels[defaultMethod]}`
    : methodLabels[method];
};
