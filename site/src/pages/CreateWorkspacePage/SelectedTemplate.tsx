import type { Template, TemplateExample } from "#/api/typesGenerated";
import { Avatar } from "#/components/Avatar/Avatar";

type SelectedTemplateProps = {
	template: Template | TemplateExample;
};

export const SelectedTemplate: React.FC<SelectedTemplateProps> = ({
	template,
}) => {
	return (
		<div className="flex gap-4 py-5 px-6 rounded-lg bg-surface-primary border">
			<Avatar
				variant="icon"
				size="lg"
				src={template.icon}
				fallback={template.name}
			/>
			<div className="flex flex-col">
				<span className="text-base">
					{"display_name" in template && template.display_name.length > 0
						? template.display_name
						: template.name}
				</span>
				{template.description && (
					<span className="text-sm text-content-secondary">
						{template.description}
					</span>
				)}
			</div>
		</div>
	);
};
