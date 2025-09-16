import Link from "@mui/material/Link";
import type { TemplateExample } from "api/typesGenerated";
import { Button } from "components/Button/Button";
import { ExternalImage } from "components/ExternalImage/ExternalImage";
import { Pill } from "components/Pill/Pill";
import type { FC, HTMLAttributes } from "react";
import { Link as RouterLink } from "react-router";
import { cn } from "utils/cn";

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
			{...divProps}
			className={cn(
				"w-80 p-6 rounded-md text-left text-inherit flex flex-col border border-border border-solid hover:border-border-hover transition-colors duration-200",
				className,
			)}
		>
			<div className="flex items-center justify-between gap-4 pb-6">
				<div className="shrink-0 pt-1 size-8">
					<ExternalImage
						src={example.icon}
						className="size-full object-contain"
					/>
				</div>

				<div className="flex flex-wrap gap-1.5 justify-end">
					{example.tags.map((tag) => (
						<RouterLink key={tag} to={`/starter-templates?tag=${tag}`}>
							<Pill className="border border-solid border-border no-underline cursor-pointer hover:border-border-hover transition-colors duration-200">
								{tag}
							</Pill>
						</RouterLink>
					))}
				</div>
			</div>

			<div>
				<h4 className="text-sm font-semibold mb-1">{example.name}</h4>
				<div className="text-sm text-content-secondary leading-normal">
					{example.description}{" "}
					<Link
						component={RouterLink}
						to={`/starter-templates/${example.id}`}
						className="inline-block text-sm mt-1"
					>
						Read more
					</Link>
				</div>
			</div>

			<div className="flex flex-col gap-3 pt-6 mt-auto items-center">
				<Button asChild className="w-full">
					<RouterLink to={`/templates/new?exampleId=${example.id}`}>
						Use template
					</RouterLink>
				</Button>
			</div>
		</div>
	);
};
