import {
  HelpTooltip,
  HelpTooltipLink,
  HelpTooltipLinksGroup,
  HelpTooltipText,
  HelpTooltipTitle,
} from "components/HelpTooltip/HelpTooltip";
import { docs } from "utils/docs";

export const Language = {
  title: "What is a group?",
  link: "User Groups",
  text:
    "Groups can be used with template RBAC to give groups of users access " +
    "to specific templates. View our docs on how to use groups.",
};

export function GroupsHelpTooltip() {
  return (
    <HelpTooltip size="small">
      <HelpTooltipTitle>{Language.title}</HelpTooltipTitle>
      <HelpTooltipText>{Language.text}</HelpTooltipText>

      <HelpTooltipLinksGroup>
        <HelpTooltipLink href={docs("/admin/groups")}>
          {Language.link}
        </HelpTooltipLink>
      </HelpTooltipLinksGroup>
    </HelpTooltip>
  );
}
