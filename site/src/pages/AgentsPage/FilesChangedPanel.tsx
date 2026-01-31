import type { FC } from "react";
import { useQuery } from "react-query";
import { chatGitChanges } from "api/queries/chats";
import { Loader } from "components/Loader/Loader";
import {
	PlusIcon,
	PencilIcon,
	Trash2Icon,
	ArrowRightLeftIcon,
	FileIcon,
} from "lucide-react";
import { cn } from "utils/cn";

interface FilesChangedPanelProps {
	chatId: string;
}

const getChangeIcon = (changeType: string) => {
	switch (changeType) {
		case "added":
			return <PlusIcon className="w-4 h-4 text-green-500" />;
		case "modified":
			return <PencilIcon className="w-4 h-4 text-yellow-500" />;
		case "deleted":
			return <Trash2Icon className="w-4 h-4 text-red-500" />;
		case "renamed":
			return <ArrowRightLeftIcon className="w-4 h-4 text-blue-500" />;
		default:
			return <FileIcon className="w-4 h-4 text-content-secondary" />;
	}
};

export const FilesChangedPanel: FC<FilesChangedPanelProps> = ({ chatId }) => {
	const gitChangesQuery = useQuery(chatGitChanges(chatId));

	if (gitChangesQuery.isLoading) {
		return <Loader size="sm" />;
	}

	const changes = gitChangesQuery.data ?? [];

	return (
		<div className="h-full flex flex-col">
			<h3 className="text-sm font-medium px-3 py-2 border-b border-border-default">
				Files Changed
			</h3>

			{changes.length === 0 ? (
				<div className="p-3 text-sm text-content-secondary">
					No file changes detected yet
				</div>
			) : (
				<ul className="flex-1 overflow-y-auto">
					{changes.map((change) => (
						<li
							key={change.id}
							className={cn(
								"flex items-start gap-2 px-3 py-2",
								"border-b border-border-default last:border-b-0",
								"hover:bg-surface-secondary transition-colors",
							)}
						>
							<span className="mt-0.5 flex-shrink-0">
								{getChangeIcon(change.change_type)}
							</span>
							<div className="min-w-0 flex-1">
								<div className="font-mono text-xs truncate">
									{change.file_path}
								</div>
								{change.diff_summary && (
									<div className="text-xs text-content-secondary mt-0.5 truncate">
										{change.diff_summary}
									</div>
								)}
								{change.old_path && (
									<div className="text-xs text-content-secondary mt-0.5 truncate">
										← {change.old_path}
									</div>
								)}
							</div>
						</li>
					))}
				</ul>
			)}
		</div>
	);
};

export default FilesChangedPanel;
