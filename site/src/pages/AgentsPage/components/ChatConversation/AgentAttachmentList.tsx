import { FileCard } from "./AttachmentBlocks";
import type { AgentAttachment } from "./messageHelpers";

type AgentAttachmentListProps = {
	attachments: readonly AgentAttachment[];
	isExpired: (fileId: string) => boolean;
};

/** Download cards for agent attachments, in the given order. */
export const AgentAttachmentList: React.FC<AgentAttachmentListProps> = ({
	attachments,
	isExpired,
}) => (
	<ul className="flex flex-col gap-2">
		{attachments.map((attachment) => (
			<li key={attachment.key}>
				<FileCard
					block={attachment.file}
					href={attachment.href}
					expired={
						attachment.file.file_id !== undefined &&
						isExpired(attachment.file.file_id)
					}
					fullWidth
				/>
			</li>
		))}
	</ul>
);
