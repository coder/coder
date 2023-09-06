import { FC } from "react";
import {
  HelpTooltip,
  HelpTooltipLink,
  HelpTooltipLinksGroup,
  HelpTooltipText,
  HelpTooltipTitle,
} from "components/HelpTooltip/HelpTooltip";
import { docs } from "utils/docs";

export const Language = {
  title: "What is a role?",
  text:
    "Coder role-based access control (RBAC) provides fine-grained access management. " +
    "View our docs on how to use the available roles.",
  link: "User Roles",
};

export const UserRoleHelpTooltip: FC = () => {
  return (
    <HelpTooltip size="small">
      <HelpTooltipTitle>{Language.title}</HelpTooltipTitle>
      <HelpTooltipText>{Language.text}</HelpTooltipText>
      <HelpTooltipLinksGroup>
        <HelpTooltipLink href={docs("/admin/users#roles")}>
          {Language.link}
        </HelpTooltipLink>
      </HelpTooltipLinksGroup>
    </HelpTooltip>
  );
};
