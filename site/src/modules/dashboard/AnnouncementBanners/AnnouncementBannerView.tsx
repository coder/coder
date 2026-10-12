import { InlineMarkdown } from "#/components/Markdown/InlineMarkdown";
import { readableForegroundColor } from "#/utils/colors";

type AnnouncementBannerViewProps = {
	message: string;
	backgroundColor: string;
};

export const AnnouncementBannerView: React.FC<AnnouncementBannerViewProps> = ({
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
