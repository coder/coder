import type { Foundation } from "#/api/typesGenerated";
import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from "#/components/Table/Table";
import { getFoundationOutputs } from "./foundationOutputs";

type FoundationResourcesProps = {
	resources: Foundation;
};

/** Shows only inventory reported by the server's retained Terraform state. */
export const FoundationResources: React.FC<FoundationResourcesProps> = ({
	resources,
}) => {
	const inventory = resources.resources ?? [];
	const outputs = getFoundationOutputs(resources);
	const inventoryUnavailable =
		resources.resources_available !== true || !resources.resources;
	return (
		<div className="space-y-8">
			<section aria-label="Managed resources" className="space-y-4">
				<div>
					<h2 className="text-xl font-semibold">Resources</h2>
					<p className="text-sm text-content-secondary">
						Managed resources from retained Terraform state, not a live cloud
						refresh.
					</p>
				</div>
				{inventoryUnavailable ? (
					<div className="rounded-lg border border-solid border-border p-8 text-center">
						<h3 className="text-lg font-semibold">
							Resource inventory unavailable
						</h3>
						<p className="text-sm text-content-secondary">
							A resource inventory is not available from this server. This does
							not mean the Foundation has no managed infrastructure.
						</p>
					</div>
				) : inventory.length === 0 ? (
					<div className="rounded-lg border border-solid border-border p-8 text-center">
						<h3 className="text-lg font-semibold">No managed resources</h3>
						<p className="text-sm text-content-secondary">
							{resources.status === "destroyed"
								? "This Foundation's infrastructure has been destroyed."
								: !resources.applied_revision_id
									? "This Foundation has not been applied. Apply a revision to provision its infrastructure."
									: "The retained Terraform state contains no managed resources."}
						</p>
					</div>
				) : (
					<Table aria-label="Managed resources">
						<TableHeader>
							<TableRow>
								<TableHead>Address</TableHead>
								<TableHead>Type</TableHead>
								<TableHead>Name</TableHead>
								<TableHead>Instances</TableHead>
							</TableRow>
						</TableHeader>
						<TableBody>
							{inventory.map((resource) => (
								<TableRow key={resource.address}>
									<TableCell className="font-mono break-all">
										{resource.address}
									</TableCell>
									<TableCell>{resource.type}</TableCell>
									<TableCell>{resource.name}</TableCell>
									<TableCell>{resource.instances}</TableCell>
								</TableRow>
							))}
						</TableBody>
					</Table>
				)}
			</section>
			<section aria-label="Terraform outputs" className="space-y-4">
				<h2 className="text-xl font-semibold">Outputs</h2>
				{resources.output_snapshot_id && (
					<p className="text-sm text-content-secondary">
						Output snapshot: {resources.output_snapshot_id}
					</p>
				)}
				{outputs.length === 0 ? (
					<p className="text-sm text-content-secondary">
						No outputs available.
					</p>
				) : (
					<Table aria-label="Terraform outputs" className="min-w-160">
						<TableHeader>
							<TableRow>
								<TableHead>Name</TableHead>
								<TableHead>Type</TableHead>
								<TableHead>Sensitivity</TableHead>
								<TableHead>Value</TableHead>
							</TableRow>
						</TableHeader>
						<TableBody>
							{outputs.map((output) => (
								<TableRow key={output.name}>
									<TableCell className="font-mono break-all">
										{output.name}
									</TableCell>
									<TableCell className="font-mono break-all">
										{output.type}
									</TableCell>
									<TableCell>{output.sensitivity}</TableCell>
									<TableCell>
										<pre className="m-0 whitespace-pre-wrap break-all text-sm">
											{output.value}
										</pre>
									</TableCell>
								</TableRow>
							))}
						</TableBody>
					</Table>
				)}
			</section>
		</div>
	);
};
