import Link from "@mui/material/Link";
import type { TemplateExample } from "api/typesGenerated";
import { Button } from "components/Button/Button";
import { CodeExample } from "components/CodeExample/CodeExample";
import { Stack } from "components/Stack/Stack";
import { TableEmpty } from "components/TableEmpty/TableEmpty";
import { TemplateExampleCard } from "modules/templates/TemplateExampleCard/TemplateExampleCard";
import type { FC } from "react";
import { Link as RouterLink } from "react-router";
import { docs } from "utils/docs";

// Those are from https://github.com/coder/coder/tree/main/examples/templates
const featuredExampleIds = [
	"tasks-docker",
	"docker",
	"kubernetes",
	"aws-linux",
	"aws-windows",
	"gcp-linux",
	"gcp-windows",
];

const findFeaturedExamples = (examples: TemplateExample[]) => {
	const featuredExamples: TemplateExample[] = [];

	// We loop the featuredExampleIds first to keep the order
	for (const exampleId of featuredExampleIds) {
		for (const example of examples) {
			if (exampleId === example.id) {
				featuredExamples.push(example);
			}
		}
	}

	return featuredExamples;
};

interface EmptyTemplatesProps {
	canCreateTemplates: boolean;
	examples: TemplateExample[];
	isUsingFilter: boolean;
}

export const EmptyTemplates: FC<EmptyTemplatesProps> = ({
	canCreateTemplates,
	examples,
	isUsingFilter,
}) => {
	if (isUsingFilter) {
		return <TableEmpty message="No results matched your search" />;
	}

	const featuredExamples = findFeaturedExamples(examples);

	if (canCreateTemplates) {
		return (
			<TableEmpty
				message="Create your first template"
				description={
					<>
						Templates are written in Terraform and describe the infrastructure
						for workspaces. You can start using a starter template below or{" "}
						<Link
							href={docs("/admin/templates/creating-templates")}
							target="_blank"
							rel="noreferrer"
						>
							create your own
						</Link>
						.
					</>
				}
				cta={
					<Stack alignItems="center" spacing={4}>
						<div className="flex flex-wrap justify-center gap-4">
							{featuredExamples.map((example) => (
								<TemplateExampleCard example={example} key={example.id} />
							))}
						</div>

						<Button size="sm" asChild className="rounded-full">
							<RouterLink to="/starter-templates">
								View all starter templates
							</RouterLink>
						</Button>
					</Stack>
				}
			/>
		);
	}

	return (
		<TableEmpty
			message="Create a Template"
			description="Contact your Coder administrator to create a template. You can share the code below."
			cta={<CodeExample secret={false} code="coder templates init" />}
		/>
	);
};
