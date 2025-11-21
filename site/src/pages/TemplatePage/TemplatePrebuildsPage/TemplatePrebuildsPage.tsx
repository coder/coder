import { API } from "api/api";
import { Button } from "components/Button/Button";
import { displaySuccess } from "components/GlobalSnackbar/utils";
import { Loader } from "components/Loader/Loader";
import { AlertTriangle, RefreshCw } from "lucide-react";
import { useTemplateLayoutContext } from "pages/TemplatePage/TemplateLayout";
import type { FC } from "react";
import { useMutation } from "react-query";
import { pageTitle } from "utils/page";

const TemplatePrebuildsPage: FC = () => {
	const { template } = useTemplateLayoutContext();

	return (
		<>
			<title>{pageTitle(`${template.name} - Prebuilds`)}</title>
			<TemplatePrebuildsPageView templateId={template.id} />
		</>
	);
};

interface TemplatePrebuildsPageViewProps {
	templateId: string;
}

const TemplatePrebuildsPageView: FC<TemplatePrebuildsPageViewProps> = ({
	templateId,
}) => {
	const invalidateMutation = useMutation({
		mutationFn: () => API.invalidateTemplatePrebuilds(templateId),
		onSuccess: (data) => {
			const count = data.invalidated_presets?.length || 0;
			displaySuccess(
				count === 1 ? "1 preset invalidated" : `${count} presets invalidated`,
			);
		},
	});

	return (
		<div className="max-w-3xl space-y-6">
			<div className="rounded-lg border border-border-default bg-surface-primary p-6">
				<div className="mb-4 flex items-start gap-3">
					<div className="flex size-10 shrink-0 items-center justify-center rounded-full bg-surface-secondary">
						<RefreshCw className="size-5 text-content-secondary" />
					</div>
					<div>
						<h2 className="mb-1 text-lg font-semibold text-content-primary">
							Invalidate Template Prebuilds
						</h2>
						<p className="text-sm text-content-secondary">
							Mark all prebuilds as expired for this template's active version.
						</p>
					</div>
				</div>

				<div className="mb-6 rounded-md bg-surface-secondary p-4">
					<div className="flex gap-3">
						<AlertTriangle className="size-5 shrink-0 text-content-secondary" />
						<div className="space-y-2 text-sm text-content-secondary">
							<p>
								<strong className="font-medium text-content-primary">
									What happens when you invalidate prebuilds:
								</strong>
							</p>
							<ul className="ml-4 list-disc space-y-1">
								<li>
									All presets for the active template version are marked with a
									new invalidation timestamp
								</li>
								<li>
									The reconciler automatically identifies expired prebuilds
									created before the invalidation time
								</li>
								<li>
									Expired prebuilds are deleted and fresh ones are created in
									the background
								</li>
								<li>
									This is useful when prebuilds become stale due to repository
									changes or infrastructure updates
								</li>
							</ul>
						</div>
					</div>
				</div>

				<Button
					onClick={() => invalidateMutation.mutate()}
					disabled={invalidateMutation.isPending}
					className="gap-2"
				>
					{invalidateMutation.isPending ? (
						<>
							<Loader className="size-4" />
							Invalidating...
						</>
					) : (
						<>
							<RefreshCw className="size-4" />
							Invalidate All Prebuilds
						</>
					)}
				</Button>

				{invalidateMutation.isError && (
					<div className="mt-4 rounded-md border border-border-error bg-surface-error p-3 text-sm text-content-error">
						Failed to invalidate prebuilds. Please try again.
					</div>
				)}
			</div>
		</div>
	);
};

export default TemplatePrebuildsPage;
