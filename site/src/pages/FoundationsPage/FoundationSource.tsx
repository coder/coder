import { useQuery } from "react-query";
import { Link } from "react-router";
import { templateArchive } from "#/api/queries/files";
import { foundationPath } from "#/api/queries/foundations";
import type { Foundation } from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Button } from "#/components/Button/Button";
import { Loader } from "#/components/Loader/Loader";
import { TemplateFileArchive } from "#/modules/templates/TemplateFiles/TemplateFiles";
import { traverse } from "#/utils/filetree";
import type { TemplateVersionFiles } from "#/utils/templateVersion";

type FoundationSourceProps = {
	resources: Foundation;
	canUpdate: boolean;
};

/** Reads the complete immutable archive without entering an editing session. */
export const FoundationSource: React.FC<FoundationSourceProps> = ({
	resources,
	canUpdate,
}) => {
	const revision = resources.revisions.find(
		(item) => item.id === resources.current_revision_id,
	);
	const query = useQuery({
		...templateArchive(revision?.file_id ?? ""),
		enabled: Boolean(revision),
	});
	const editHref = `${foundationPath(resources.organization_id, resources.id)}?edit=true`;
	const files: TemplateVersionFiles = {};
	if (query.data) {
		traverse(query.data.files, (content, _name, path) => {
			if (typeof content === "string") files[path] = content;
		});
	}
	return (
		<>
			{Boolean(query.error) && (
				<div className="mb-6 space-y-3">
					<ErrorAlert error={query.error} />
					<Button variant="outline" onClick={() => void query.refetch()}>
						Retry files
					</Button>
				</div>
			)}
			{!revision ? (
				<div className="rounded-lg border border-solid border-border p-8 text-center space-y-4">
					<h2 className="text-lg font-semibold">No source code yet</h2>
					<p className="text-sm text-content-secondary">
						Save your first immutable revision in the file editor.
					</p>
					{canUpdate && (
						<Button asChild>
							<Link to={editHref}>Edit files</Link>
						</Button>
					)}
				</div>
			) : query.isLoading ? (
				<Loader />
			) : (
				query.data &&
				(Object.keys(files).length === 0 ? (
					<p className="text-content-secondary">
						This archive contains no text files.
					</p>
				) : (
					<TemplateFileArchive
						currentFiles={files}
						editFileHref={
							canUpdate
								? (path) => `${editHref}&path=${encodeURIComponent(path)}`
								: undefined
						}
					/>
				))
			)}
		</>
	);
};
