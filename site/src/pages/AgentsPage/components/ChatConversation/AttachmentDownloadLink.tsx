import { handleAttachmentDownloadClick } from "../../utils/chatAttachments";

/** `downloadName` names the saved file, which can differ from `displayName`. */
export type AttachmentDownload = {
	href: string;
	displayName: string;
	downloadName: string;
	mediaType: string;
};

type AttachmentDownloadLinkProps = React.PropsWithChildren<
	AttachmentDownload & {
		className: string;
		ref?: React.Ref<HTMLAnchorElement>;
	}
>;

/** Downloads the file, or opens the share sheet in an installed iOS app. */
export const AttachmentDownloadLink: React.FC<AttachmentDownloadLinkProps> = ({
	href,
	displayName,
	downloadName,
	mediaType,
	className,
	ref,
	children,
}) => (
	<a
		ref={ref}
		href={href}
		download={downloadName}
		onClick={(event) => {
			event.stopPropagation();
			void handleAttachmentDownloadClick(event, {
				href,
				fileName: downloadName,
				mediaType,
			});
		}}
		aria-label={`Download ${displayName}`}
		className={className}
	>
		{children}
	</a>
);
