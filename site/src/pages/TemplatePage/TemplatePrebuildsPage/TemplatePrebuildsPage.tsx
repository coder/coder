import { API } from "api/api";
import type { InvalidatePresetsResponse } from "api/typesGenerated";
import { Button } from "components/Button/Button";
import { displayError, displaySuccess } from "components/GlobalSnackbar/utils";
import { Loader } from "components/Loader/Loader";
import { RefreshCw } from "lucide-react";
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
		mutationFn: () => API.invalidateTemplatePresets(templateId),
		onSuccess: (data: InvalidatePresetsResponse) => {
			if (data.invalidated.length === 0) {
				displaySuccess("No presets required invalidation.");
				return;
			}

			// They all have the same template name/version
			const { template_name, template_version_name } = data.invalidated[0];
			// List only preset names
			const presets = data.invalidated
				.map((p) => `• ${p.preset_name}`)
				.join("\n");

			displaySuccess(
				`Invalidated presets for template "${template_name}" (version ${template_version_name}):\n${presets}`,
			);
		},
		onError: () => displayError("Failed to invalidate template presets."),
	});

	return (
		<div className="flex">
			<div className="max-w-xl space-y-6">
				<div>
					<h3 className="text-xl text-content-primary m-0">
						Invalidate presets
					</h3>
					<p className="text-sm text-content-secondary">
						All template presets for the active template version are marked with
						a new invalidation timestamp. The reconciler automatically
						identifies expired prebuilds created before the invalidation time.
						This is useful when prebuilds become stale due to repository changes
						or infrastructure updates and need recycling.
					</p>
				</div>

				<div>
					<Button
						onClick={() => invalidateMutation.mutate()}
						disabled={invalidateMutation.isPending}
						className="gap-2"
					>
						<RefreshCw className="size-4" />
						Invalidate now
					</Button>
				</div>
			</div>
		</div>
	);
};

export default TemplatePrebuildsPage;
