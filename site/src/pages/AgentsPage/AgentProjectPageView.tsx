import type { FC } from "react";
import { Link } from "react-router";
import type { Chat, ChatProject } from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Button } from "#/components/Button/Button";
import { Skeleton } from "#/components/Skeleton/Skeleton";
import { buildAgentChatPath } from "./utils/navigation";

type AgentProjectPageViewProps = {
	readonly project?: ChatProject;
	readonly chats: readonly Chat[];
	readonly isLoading: boolean;
	readonly error?: unknown;
	readonly onEdit: () => void;
	readonly newChatPath: string;
};

export const AgentProjectPageView: FC<AgentProjectPageViewProps> = ({
	project,
	chats,
	isLoading,
	error,
	onEdit,
	newChatPath,
}) => {
	if (isLoading) {
		return (
			<div className="space-y-4 p-6">
				<Skeleton className="h-7 w-48" />
				<Skeleton className="h-4 w-80" />
				<Skeleton className="h-12 w-full" />
			</div>
		);
	}

	if (error) {
		return <ErrorAlert error={error} className="m-6" />;
	}

	if (!project) {
		return (
			<div className="p-6 text-sm text-content-secondary">
				Project not found.
			</div>
		);
	}

	return (
		<div className="flex min-h-0 flex-1 flex-col overflow-auto p-6">
			<div className="flex items-start justify-between gap-4">
				<div className="min-w-0">
					<h1 className="m-0 text-xl font-semibold text-content-primary">
						{project.name}
					</h1>
					{project.description && (
						<p className="mb-0 mt-2 text-sm text-content-secondary">
							{project.description}
						</p>
					)}
				</div>
				<div className="flex shrink-0 gap-2">
					<Button variant="outline" onClick={onEdit}>
						Edit
					</Button>
					<Button asChild>
						<Link to={newChatPath}>New chat</Link>
					</Button>
				</div>
			</div>
			<div className="mt-8 flex flex-col gap-1">
				{chats.length === 0 ? (
					<p className="text-sm text-content-secondary">
						No chats in this project yet
					</p>
				) : (
					chats.map((chat) => (
						<Link
							key={chat.id}
							to={buildAgentChatPath({ chatId: chat.id })}
							className="rounded-md px-3 py-2 text-sm text-content-primary no-underline hover:bg-surface-secondary"
						>
							{chat.title || "Untitled"}
						</Link>
					))
				)}
			</div>
		</div>
	);
};
