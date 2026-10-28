import type { Foundation } from "#/api/typesGenerated";
import { Badge } from "#/components/Badge/Badge";
import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from "#/components/Table/Table";
import { SharedResourceEditor } from "#/pages/TemplatePage/TemplateSharedResourcesPage/SharedResourceEditor";

type FoundationVersionsProps = React.ComponentProps<
	typeof SharedResourceEditor
>;

/** Pairs immutable revision history with guarded lifecycle operations. */
export const FoundationVersions: React.FC<FoundationVersionsProps> = (
	props,
) => {
	const { resources } = props;
	const revisions: Foundation["revisions"] = resources.revisions.toSorted(
		(a, b) => b.created_at.localeCompare(a.created_at),
	);
	return (
		<div className="grid gap-8 xl:grid-cols-[minmax(0,1fr)_320px]">
			<section aria-label="Foundation versions" className="space-y-4">
				<h2 className="text-xl font-semibold">Versions</h2>
				{revisions.length === 0 ? (
					<p className="text-sm text-content-secondary">
						No saved revisions. Save a revision in Edit files to get started.
					</p>
				) : (
					<Table aria-label="Foundation revisions">
						<TableHeader>
							<TableRow>
								<TableHead>Revision</TableHead>
								<TableHead>Created</TableHead>
								<TableHead>Status</TableHead>
							</TableRow>
						</TableHeader>
						<TableBody>
							{revisions.map((revision) => (
								<TableRow key={revision.id}>
									<TableCell className="font-mono" title={revision.id}>
										{revision.id.slice(0, 8)}
									</TableCell>
									<TableCell>
										{new Date(revision.created_at).toLocaleString("en-US")}
									</TableCell>
									<TableCell>
										<div className="flex flex-wrap gap-2">
											{revision.id === resources.current_revision_id && (
												<Badge variant="outline">Current</Badge>
											)}
											{revision.id === resources.applied_revision_id && (
												<Badge variant="green">Applied</Badge>
											)}
										</div>
									</TableCell>
								</TableRow>
							))}
						</TableBody>
					</Table>
				)}
			</section>
			<SharedResourceEditor {...props} />
		</div>
	);
};
