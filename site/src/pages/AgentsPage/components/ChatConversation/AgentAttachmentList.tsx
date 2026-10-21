import { FileCard } from "./AttachmentBlocks";
import type { AgentAttachment } from "./messageHelpers";

type AgentAttachmentListProps = {
	attachments: readonly AgentAttachment[];
	isExpired: (fileId: string) => boolean;
};

/** Full-width download cards for agent attachments, in the given order. */
export const AgentAttachmentList: React.FC<AgentAttachmentListProps> = ({
	attachments,
	isExpired,
}) => (
	<ul className="flex flex-col gap-2">
		{attachments.map(({ key, file, href }) => (
			<li key={key}>
				<FileCard
					block={file}
					href={href}
					expired={file.file_id !== undefined && isExpired(file.file_id)}
					fullWidth
				/>
			</li>
		))}
	</ul>
);
