import type { SvgIconProps } from "@mui/material/SvgIcon";
import { ExternalImage } from "components/ExternalImage/ExternalImage";
import { BookOpenTextIcon, BugIcon, MessageSquareIcon } from "lucide-react";
import type { FC } from "react";

interface SupportIconProps {
	icon: string;
	className?: string;
}

export const SupportIcon: FC<SupportIconProps> = ({ icon, className }) => {
	switch (icon) {
		case "bug":
			return <BugIcon className={className} />;
		case "chat":
			return <MessageSquareIcon className={className} />;
		case "docs":
			return <BookOpenTextIcon className={className} />;
		case "star":
			return <GithubStar className={className} />;
		default:
			return <ExternalImage src={icon} className={className} />;
	}
};

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
