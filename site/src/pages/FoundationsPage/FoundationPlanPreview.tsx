import type { Foundation, TemplateResourceRun } from "#/api/typesGenerated";

/** Selects only the newest Plan preview for the revision being applied. */
export const getFoundationPlanPreview = (
	resources: Foundation,
	revisionId: string | undefined,
) =>
	resources.runs
		.filter((run) => run.action === "plan" && run.revision_id === revisionId)
		.toSorted((a, b) => b.created_at.localeCompare(a.created_at))[0]?.preview;

type FoundationPlanPreviewProps = { preview: TemplateResourceRun["preview"] };

/** Displays value-free advisory metadata, never treating a preview as approval. */
export const FoundationPlanPreview: React.FC<FoundationPlanPreviewProps> = ({
	preview,
}) => (
	<section aria-label="Plan impact preview" className="space-y-3 text-sm">
		<h3 className="font-semibold">Plan impact preview</h3>
		{!preview ? (
			<p>
				No Plan preview is recorded for this revision. Apply does not require a
				prior Plan.
			</p>
		) : !preview.available ? (
			<p>
				Plan preview could not be read or is unsupported. This does not mean
				there are no changes.
			</p>
		) : preview.output_changes.length === 0 ? (
			<p>
				No output changes reported by this preview. Infrastructure may still
				change.
			</p>
		) : (
			<ul className="space-y-1 pl-5">
				{preview.output_changes.map((change) => (
					<li key={change.name}>
						<span className="font-mono">{change.name}</span>:{" "}
						{change.actions.join(", ") || "Unknown action"}
						{change.sensitive ? " (sensitive, values redacted)" : ""}
						{change.unknown ? " (value unknown until Apply)" : ""}
					</li>
				))}
			</ul>
		)}
		{preview?.consumer_references.map((reference) => (
			<div key={reference.template_version_id} className="space-y-1">
				{reference.output_names.length > 0 && (
					<p className="text-content-destructive">
						Template version {reference.template_version_id} references changed
						or removed outputs: {reference.output_names.join(", ")}. These
						changes may break consumers.
					</p>
				)}
				{reference.unresolved && (
					<p>
						Template version {reference.template_version_id}: some output
						references could not be resolved. Dynamic references or unavailable
						source prevent a complete assessment.
					</p>
				)}
			</div>
		))}
		<p className="text-content-secondary">
			{preview?.advisory ||
				"This advisory preview is not a safety guarantee. Unchanged outputs do not guarantee workspace compatibility."}
		</p>
		<p className="text-content-secondary">
			Apply creates a fresh plan and may differ from this preview.
		</p>
	</section>
);
