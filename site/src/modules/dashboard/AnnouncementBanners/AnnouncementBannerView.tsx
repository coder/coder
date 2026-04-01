import type { FC } from "react";
import { InlineMarkdown } from "#/components/Markdown/Markdown";
import { readableForegroundColor } from "#/utils/colors";

interface AnnouncementBannerViewProps {
	message: string;
	backgroundColor: string;
}

export const AnnouncementBannerView: FC<AnnouncementBannerViewProps> = ({
	message,
	backgroundColor,
}) => {
	return (
		<div
			className="p-3 flex items-center"
			style={{ backgroundColor }}
			data-test-id="service-banner"
		>
			<div
				className="mx-auto font-normal [&_a]:text-inherit [&_a]:underline"
				style={{ color: readableForegroundColor(backgroundColor) }}
			>
				<InlineMarkdown>{message}</InlineMarkdown>
			</div>
		</div>
	);
};
