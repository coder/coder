import { type FC, useEffect, useState } from "react";
import { useQuery } from "react-query";
import { API } from "#/api/api";
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogHeader,
	DialogTitle,
} from "#/components/Dialog/Dialog";
import { Badge } from "#/components/Badge/Badge";
import { Loader } from "#/components/Loader/Loader";
import { Markdown } from "#/components/Markdown/Markdown";

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

	const [imageObjectURL, setImageObjectURL] = useState<string | null>(null);

	useEffect(() => {
		if (!data?.image_url) {
			setImageObjectURL(null);
			return;
		}

		let cancelled = false;
		let objectURL: string | null = null;

		void (async () => {
			try {
				const blob = await API.getChangelogAsset(data.image_url);
				if (cancelled) {
					return;
				}
				objectURL = URL.createObjectURL(blob);
				setImageObjectURL(objectURL);
			} catch {
				if (!cancelled) {
					setImageObjectURL(null);
				}
			}
		})();

		return () => {
			cancelled = true;
			if (objectURL) {
				URL.revokeObjectURL(objectURL);
			}
		};
	}, [data?.image_url]);

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
								<Badge
									variant="info"
									size="xs"
									border="solid"
									className="font-semibold"
								>
									Changelog
								</Badge>
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
								src={imageObjectURL ?? data.image_url}
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
