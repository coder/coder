import { FileCard } from "./AttachmentBlocks";
import type { AgentAttachment } from "./messageHelpers";

/** Inline attachments carry no file ID, so only stored files can expire. */
const isAgentAttachmentExpired = (
	{ file }: AgentAttachment,
	isExpired: (fileId: string) => boolean,
): boolean => file.file_id !== undefined && isExpired(file.file_id);

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
					expired={isAgentAttachmentExpired(attachment, isExpired)}
					fullWidth
				/>
			</li>
		))}
	</ul>
);
