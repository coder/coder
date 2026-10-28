import type { Foundation } from "#/api/typesGenerated";

type FoundationImpactProps = { resources: Foundation };

/** Reports the server's complete consumer counts without implying compatibility. */
export const FoundationImpact: React.FC<FoundationImpactProps> = ({
	resources,
}) => {
	const counts = resources.consumers;
	return (
		<section className="space-y-3" aria-label="Consumer impact">
			<h2 className="text-lg font-semibold">Consumer impact</h2>
			{counts ? (
				<p className="text-sm">
					{counts.total} workspaces: {counts.running} running, {counts.stopped}{" "}
					stopped, {counts.unknown} unknown or transitioning.
				</p>
			) : (
				<p className="text-sm">Consumer counts are unavailable.</p>
			)}
			<p className="text-sm text-content-secondary">
				Counts are advisory and may change. Changes affect live shared
				infrastructure for all consumers. Workspace compatibility is not
				assessed, even if outputs are unchanged. No workspace is rebuilt or
				upgraded automatically.
			</p>
		</section>
	);
};
