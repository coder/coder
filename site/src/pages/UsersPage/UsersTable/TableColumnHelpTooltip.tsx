import { FC } from "react";
import {
  HelpTooltip,
  HelpTooltipLink,
  HelpTooltipLinksGroup,
  HelpTooltipText,
  HelpTooltipTitle,
} from "components/HelpTooltip/HelpTooltip";
import { docs } from "utils/docs";

type ColumnHeader = "roles" | "groups";

type TooltipData = {
  title: string;
  text: string;
  links: readonly { text: string; href: string }[];
};

export const Language = {
  roles: {
    title: "What is a role?",
    text:
      "Coder role-based access control (RBAC) provides fine-grained access management. " +
      "View our docs on how to use the available roles.",
    links: [{ text: "User Roles", href: docs("/admin/users#roles") }],
  },

  groups: {
    title: "What is a group?",
    text:
      "Groups can be used with template RBAC to give groups of users access " +
      "to specific templates. View our docs on how to use groups.",
    links: [{ text: "User Groups", href: docs("/admin/groups") }],
  },
} as const satisfies Record<ColumnHeader, TooltipData>;

type Props = {
  variant: ColumnHeader;
};

export const TableColumnHelpTooltip: FC<Props> = ({ variant }) => {
  const variantLang = Language[variant];

  return (
    <HelpTooltip size="small">
      <HelpTooltipTitle>{variantLang.title}</HelpTooltipTitle>
      <HelpTooltipText>{variantLang.text}</HelpTooltipText>

      <HelpTooltipLinksGroup>
        {variantLang.links.map((link) => (
          <HelpTooltipLink key={link.text} href={link.href}>
            {link.text}
          </HelpTooltipLink>
        ))}
      </HelpTooltipLinksGroup>
    </HelpTooltip>
  );
};
