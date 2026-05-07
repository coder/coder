import Link from "@mui/material/Link";
import type { FC, HTMLAttributes } from "react";
import { Link as RouterLink } from "react-router";
import type { TemplateExample } from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import { ExternalImage } from "#/components/ExternalImage/ExternalImage";
import { Pill } from "#/components/Pill/Pill";
import { cn } from "#/utils/cn";

type TemplateExampleCardProps = HTMLAttributes<HTMLDivElement> & {
	example: TemplateExample;
	activeTag?: string;
};

export const TemplateExampleCard: FC<TemplateExampleCardProps> = ({
	example,
	activeTag,
	className,
	...divProps
}) => {
	return (
		<div
			className={cn(
				"flex w-[320px] flex-col rounded-md border border-solid border-border p-6 text-left text-inherit",
				className,
			)}
			{...divProps}
		>
			<div className="mb-6 flex items-center justify-between">
				<div className="size-8 shrink-0 pt-1">
					<ExternalImage
						src={example.icon}
						className="w-full h-full object-contain"
					/>
				</div>

				<div className="flex flex-wrap justify-end gap-2">
					{example.tags.map((tag) => (
						<RouterLink key={tag} to={`/starter-templates?tag=${tag}`}>
							<Pill
								className={cn(
									"cursor-pointer border-border",
									"hover:border-content-primary",
									activeTag === tag && "!border-border-pending !bg-surface-sky",
								)}
							>
								{tag}
							</Pill>
						</RouterLink>
					))}
				</div>
			</div>

			<div>
				<h4 className="text-sm font-semibold m-0 mb-1">{example.name}</h4>
				<span className="block text-[13px] leading-[1.6] text-content-secondary">
					{example.description}{" "}
					<Link
						component={RouterLink}
						to={`/starter-templates/${example.id}`}
						className="inline-block text-[13px] mt-1"
					>
						Read more
					</Link>
				</span>
			</div>

			<div className="mt-auto flex flex-col items-center gap-3 pt-6">
				<Button asChild className="w-full">
					<RouterLink to={`/templates/new?exampleId=${example.id}`}>
						Use template
					</RouterLink>
				</Button>
			</div>
		</div>
	);
};
