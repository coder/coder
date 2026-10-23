import type { TemplateResources } from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Badge } from "#/components/Badge/Badge";
import { Button } from "#/components/Button/Button";
import { Loader } from "#/components/Loader/Loader";
import { TemplateResourceEditor } from "./TemplateResourceEditor";
import { TemplateResourceRunHistory } from "./TemplateResourceRunHistory";

type TemplateTerraformResourcesPageViewProps = Omit<
	React.ComponentProps<typeof TemplateResourceEditor>,
	"resources"
> & {
	resources: TemplateResources | undefined;
	error: unknown;
	isLoading: boolean;
	onRetry: () => void;
};

/** Keeps the editor and persisted state visible during background refreshes. */
export const TemplateTerraformResourcesPageView: React.FC<
	TemplateTerraformResourcesPageViewProps
> = ({ resources, error, isLoading, onRetry, ...editorProps }) => {
	return (
		<div className="space-y-8">
			<header className="space-y-2">
				<h1 className="text-2xl font-semibold">Template resources</h1>
				<p className="text-content-secondary">
					Manage any Terraform resources once per template in an independent
					root with its own state. Save and apply revisions separately from
					workspace version publishing and workspace builds.
				</p>
			</header>
			{Boolean(error) && (
				<div className="space-y-3">
					<ErrorAlert error={error} />
					<Button variant="outline" onClick={onRetry}>
						Retry resources
					</Button>
				</div>
			)}
			{isLoading && !resources && <Loader />}
			{resources && (
				<>
					<section
						aria-label="Resource state"
						className="rounded-lg border border-border p-4 space-y-4"
					>
						<h2 className="text-xl font-semibold">Current state</h2>
						<dl className="grid gap-4 sm:grid-cols-3 text-sm">
							<div>
								<dt className="text-content-secondary mb-2">Status</dt>
								<dd className="m-0">
									<Badge
										variant={
											resources.status === "failed"
												? "destructive"
												: resources.status === "applied"
													? "green"
													: "outline"
										}
									>
										{resources.status}
									</Badge>
								</dd>
							</div>
							<div>
								<dt className="text-content-secondary mb-2">
									Current revision
								</dt>
								<dd className="m-0 break-all font-mono">
									{resources.current_revision_id ?? "None"}
								</dd>
							</div>
							<div>
								<dt className="text-content-secondary mb-2">
									Applied revision
								</dt>
								<dd className="m-0 break-all font-mono">
									{resources.applied_revision_id ?? "None"}
								</dd>
							</div>
						</dl>
						{resources.status === "unconfigured" && (
							<p className="text-sm text-content-secondary">
								{resources.revisions.length === 0
									? "No template resources configured. Open Edit files and build your first revision to get started."
									: "Your saved revision is ready to plan or apply."}
							</p>
						)}
						{resources.status === "unknown" && (
							<p className="text-sm text-content-secondary">
								The resource state is unknown. Review the latest job logs.
								Manual reconciliation is required before Terraform can run
								again.
							</p>
						)}
					</section>
					<TemplateResourceEditor
						key={resources.template_id}
						resources={resources}
						{...editorProps}
					/>
					<section aria-label="Terraform outputs" className="space-y-3">
						<h2 className="text-xl font-semibold">Outputs</h2>
						{Object.keys(resources.outputs).length === 0 ? (
							<p className="text-sm text-content-secondary">
								No outputs available.
							</p>
						) : (
							<pre className="overflow-auto rounded-lg border border-border bg-surface-secondary p-4 text-sm">
								{JSON.stringify(resources.outputs, null, 2)}
							</pre>
						)}
					</section>
					<TemplateResourceRunHistory
						key={`runs-${resources.template_id}`}
						resources={resources}
					/>
				</>
			)}
		</div>
	);
};
