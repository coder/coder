import type { FC } from "react";
import type { Template, TemplateExample } from "#/api/typesGenerated";
import { Avatar } from "#/components/Avatar/Avatar";
import { Stack } from "#/components/Stack/Stack";

interface SelectedTemplateProps {
	template: Template | TemplateExample;
}

export const SelectedTemplate: FC<SelectedTemplateProps> = ({ template }) => {
	return (
		<Stack
			direction="row"
			className="py-5 px-6 rounded-lg bg-surface-primary border border-solid border-border"
		>
			<Avatar
				variant="icon"
				size="lg"
				src={template.icon}
				fallback={template.name}
			/>
			<Stack direction="column" spacing={0}>
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
			</Stack>
		</Stack>
	);
};
