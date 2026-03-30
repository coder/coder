import type { FC } from "react";
import {
	HelpPopover,
	HelpPopoverContent,
	HelpPopoverIconTrigger,
	HelpPopoverLink,
	HelpPopoverLinksGroup,
	HelpPopoverText,
	HelpPopoverTitle,
} from "#/components/HelpPopover/HelpPopover";
import { docs } from "#/utils/docs";

type ColumnHeader = "roles" | "groups" | "ai_addon";

type TooltipData = {
	title: string;
	text: string;
	links: readonly { text: string; href: string }[];
};

const Language = {
	roles: {
		title: "What is a role?",
		text:
			"Coder role-based access control (RBAC) provides fine-grained access management. " +
			"View our docs on how to use the available roles.",
		links: [{ text: "User Roles", href: docs("/admin/users/groups-roles") }],
	},

	groups: {
		title: "What is a group?",
		text:
			"Groups can be used with template RBAC to give groups of users access " +
			"to specific templates. View our docs on how to use groups.",
		links: [{ text: "User Groups", href: docs("/admin/users/groups-roles") }],
	},

	ai_addon: {
		title: "What is the AI add-on?",
		text:
			"Users with access to AI features like AI Bridge or Tasks " +
			"who are actively consuming a seat.",
		links: [],
	},
} as const satisfies Record<ColumnHeader, TooltipData>;

type Props = {
	variant: ColumnHeader;
};

export const TableColumnHelpPopover: FC<Props> = ({ variant }) => {
	const variantLang = Language[variant];

	return (
		<HelpPopover>
			<HelpPopoverIconTrigger size="small" />
			<HelpPopoverContent>
				<HelpPopoverTitle>{variantLang.title}</HelpPopoverTitle>
				<HelpPopoverText>{variantLang.text}</HelpPopoverText>
				{variantLang.links.length > 0 && (
					<HelpPopoverLinksGroup>
						{variantLang.links.map((link) => (
							<HelpPopoverLink key={link.text} href={link.href}>
								{link.text}
							</HelpPopoverLink>
						))}
					</HelpPopoverLinksGroup>
				)}
			</HelpPopoverContent>
		</HelpPopover>
	);
};
