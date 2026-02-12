import { templateVersions } from "api/queries/templates";
import { Badge } from "components/Badge/Badge";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuRadioGroup,
	DropdownMenuRadioItem,
	DropdownMenuTrigger,
} from "components/DropdownMenu/DropdownMenu";
import { Skeleton } from "components/Skeleton/Skeleton";
import {
	Tooltip,
	TooltipContent,
	TooltipProvider,
	TooltipTrigger,
} from "components/Tooltip/Tooltip";
import { ChevronDownIcon } from "lucide-react";
import type { FC } from "react";
import { useQuery } from "react-query";

type TemplateVersionSelectProps = {
	value: string;
	templateId: string;
	activeVersionId: string;
	onValueChange: (value: string) => void;
};

export const TemplateVersionSelect: FC<TemplateVersionSelectProps> = ({
	templateId,
	activeVersionId,
	value,
	onValueChange,
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

	const selectedVersion = versions.find((v) => v.id === value);

	return (
		<DropdownMenu>
			<TooltipProvider>
				<Tooltip>
					<TooltipTrigger asChild>
						<DropdownMenuTrigger
							className="w-full md:w-auto max-w-full overflow-hidden border-0 bg-surface-secondary text-sm text-content-primary gap-2 px-4 md:px-3
								[&_svg]:text-inherit cursor-pointer hover:bg-surface-quaternary rounded-full
								h-10 md:h-8 data-[state=open]:bg-surface-tertiary
								flex items-center"
						>
							<span className="overflow-hidden min-w-0 flex items-center gap-2">
								<span className="truncate">
									{selectedVersion?.name ?? "Select a version"}
								</span>
							</span>
							<ChevronDownIcon className="size-icon-sm shrink-0" />
						</DropdownMenuTrigger>
					</TooltipTrigger>
					<TooltipContent>Template version</TooltipContent>
				</Tooltip>
			</TooltipProvider>
			<DropdownMenuContent align="start">
				<DropdownMenuRadioGroup value={value} onValueChange={onValueChange}>
					{versions.map((version) => {
						return (
							<DropdownMenuRadioItem value={version.id} key={version.id}>
								<span className="flex items-center gap-2 min-w-0">
									<span className="truncate">{version.name}</span>
									{activeVersionId === version.id && (
										<Badge size="xs" variant="green" className="shrink-0">
											Active
										</Badge>
									)}
								</span>
							</DropdownMenuRadioItem>
						);
					})}
				</DropdownMenuRadioGroup>
			</DropdownMenuContent>
		</DropdownMenu>
	);
};
