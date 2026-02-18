import { API } from "api/api";
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogHeader,
	DialogTitle,
} from "components/Dialog/Dialog";
import { Loader } from "components/Loader/Loader";
import { Markdown } from "components/Markdown/Markdown";
import type { FC } from "react";
import { useQuery } from "react-query";

interface ChangelogDialogProps {
	version: string | null;
	onClose: () => void;
}

export const ChangelogDialog: FC<ChangelogDialogProps> = ({
	version,
	onClose,
}) => {
	const { data, error, isLoading } = useQuery({
		queryKey: ["changelog", version],
		queryFn: () => API.getChangelogEntry(version!),
		enabled: Boolean(version),
	});

	return (
		<Dialog open={Boolean(version)} onOpenChange={(open) => !open && onClose()}>
			<DialogContent className="max-w-2xl max-h-[80vh] overflow-y-auto">
				{isLoading ? (
					<Loader />
				) : error ? (
					<div className="text-sm text-content-secondary">
						Unable to load changelog.
					</div>
				) : data ? (
					<>
						<DialogHeader>
							<div className="flex items-center gap-2 mb-2">
								<span className="inline-flex items-center px-2 py-0.5 rounded-md text-xs font-semibold bg-violet-100 text-violet-700 dark:bg-violet-500/20 dark:text-violet-400 border border-violet-300 dark:border-violet-500/30">
									Changelog
								</span>
								<span className="inline-flex items-center px-2 py-0.5 rounded-md text-xs font-medium bg-surface-secondary text-content-secondary border border-default">
									{data.version}
								</span>
								<span className="text-xs text-content-secondary">
									{data.date}
								</span>
							</div>
							<DialogTitle>{data.title}</DialogTitle>
							{data.summary && (
								<DialogDescription>{data.summary}</DialogDescription>
							)}
						</DialogHeader>

						{data.image_url && (
							<img
								src={data.image_url}
								alt={`${data.title} hero`}
								className="w-full rounded-lg"
							/>
						)}

						<Markdown className="prose prose-sm dark:prose-invert max-w-none">
							{data.content ?? ""}
						</Markdown>
					</>
				) : null}
			</DialogContent>
		</Dialog>
	);
};
