// components/Navbar/SupportButtons.tsx

import type { Interpolation, Theme } from "@emotion/react";
import type { SvgIconProps } from "@mui/material/SvgIcon";
import type * as TypesGen from "api/typesGenerated";
import { Button } from "components/Button/Button";
import { ExternalImage } from "components/ExternalImage/ExternalImage";
import { BookOpenTextIcon, BugIcon, MessageSquareIcon } from "lucide-react";
import type { FC, JSX } from "react";

interface SupportButtonsProps {
	supportLinks: TypesGen.LinkConfig[];
}

export const SupportButtons: FC<SupportButtonsProps> = ({ supportLinks }) => {
	return (
		<>
			{supportLinks.map((link) => (
				<a
					key={link.name}
					href={link.target}
					target="_blank"
					rel="noreferrer"
					className="inline-block"
				>
					<Button variant="outline">
						{link.icon !== "" &&
							renderSupportIcon(link.icon, styles.buttonIcon)}
						{link.name}
					</Button>
				</a>
			))}
		</>
	);
};

export function renderSupportIcon(
	icon: string,
	iconCss?: Interpolation<Theme>,
): JSX.Element {
	switch (icon) {
		case "bug":
			return <BugIcon css={iconCss} />;
		case "chat":
			return <MessageSquareIcon css={iconCss} />;
		case "docs":
			return <BookOpenTextIcon css={iconCss} />;
		case "star":
			return <GithubStar css={iconCss} />;
		default:
			return (
				<ExternalImage
					src={icon}
					css={{ maxWidth: "20px", maxHeight: "20px" }}
				/>
			);
	}
}

const GithubStar: FC<SvgIconProps> = (props) => (
	<svg
		aria-hidden="true"
		height="16"
		viewBox="0 0 16 16"
		version="1.1"
		width="16"
		data-view-component="true"
		fill="currentColor"
		{...props}
	>
		<path d="M8 .25a.75.75 0 0 1 .673.418l1.882 3.815 4.21.612a.75.75 0 0 1 .416 1.279l-3.046 2.97.719 4.192a.751.751 0 0 1-1.088.791L8 12.347l-3.766 1.98a.75.75 0 0 1-1.088-.79l.72-4.194L.818 6.374a.75.75 0 0 1 .416-1.28l4.21-.611L7.327.668A.75.75 0 0 1 8 .25Z" />
	</svg>
);

const styles = {
	buttonIcon: (theme) => ({
		color: theme.palette.text.secondary,
		width: 20,
		height: 20,
	}),
} satisfies Record<string, Interpolation<Theme>>;
