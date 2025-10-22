import { templateVersions } from "api/queries/templates";
import { Badge } from "components/Badge/Badge";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectValue,
} from "components/Select/Select";
import { Skeleton } from "components/Skeleton/Skeleton";
import type { FC } from "react";
import { useQuery } from "react-query";
import { PromptSelectTrigger } from "./PromptSelectTrigger";

type TemplateVersionSelectProps = {
	value: string;
	templateId: string;
	activeVersionId: string;
	onValueChange: (value: string) => void;
};

export const TemplateVersionSelect: FC<TemplateVersionSelectProps> = ({
	templateId,
	activeVersionId,
	...props
}) => {
	const { data: versions } = useQuery({
		...templateVersions(templateId),
		select: (versions) =>
			versions
				.filter((v) => !v.archived)
				.toSorted((a, b) => {
					return (
						new Date(b.created_at).getTime() - new Date(a.created_at).getTime()
					);
				}),
	});

	if (!versions) {
		return <Skeleton className="w-28 h-8 rounded-full" />;
	}

	return (
		<Select name="versionId" {...props}>
			<PromptSelectTrigger id="versionId" tooltip="Template version">
				<SelectValue placeholder="Select a version" />
			</PromptSelectTrigger>
			<SelectContent>
				{versions.map((version) => {
					return (
						<SelectItem value={version.id} key={version.id}>
							<span className="flex items-center gap-2">
								{version.name}
								{activeVersionId === version.id && (
									<Badge size="xs" variant="green">
										Active
									</Badge>
								)}
							</span>
						</SelectItem>
					);
				})}
			</SelectContent>
		</Select>
	);
};
