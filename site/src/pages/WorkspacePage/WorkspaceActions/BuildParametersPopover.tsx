import { useTheme } from "@emotion/react";
import visuallyHidden from "@mui/utils/visuallyHidden";
import { richParameters } from "api/queries/templates";
import { workspaceBuildParameters } from "api/queries/workspaceBuilds";
import type {
	TemplateVersionParameter,
	Workspace,
	WorkspaceBuildParameter,
} from "api/typesGenerated";
import { Button } from "components/Button/Button";
import { FormFields } from "components/Form/Form";
import { TopbarButton } from "components/FullPageLayout/Topbar";
import {
	HelpTooltipLink,
	HelpTooltipLinksGroup,
	HelpTooltipText,
	HelpTooltipTitle,
} from "components/HelpTooltip/HelpTooltip";
import { Link } from "components/Link/Link";
import { Loader } from "components/Loader/Loader";
import {
	Popover,
	PopoverContent,
	PopoverTrigger,
} from "components/Popover/Popover";
import { RichParameterInput } from "components/RichParameterInput/RichParameterInput";
import { useFormik } from "formik";
import { ChevronDownIcon } from "lucide-react";
import { type FC, useState } from "react";
import { useQuery } from "react-query";
import { docs } from "utils/docs";
import { getFormHelpers } from "utils/formUtils";
import {
	type AutofillBuildParameter,
	getInitialRichParameterValues,
} from "utils/richParameters";

interface BuildParametersPopoverProps {
	workspace: Workspace;
	disabled?: boolean;
	onSubmit: (buildParameters: WorkspaceBuildParameter[]) => void;
	label: string;
}

export const BuildParametersPopover: FC<BuildParametersPopoverProps> = ({
	workspace,
	disabled,
	label,
	onSubmit,
}) => {
	const [isOpen, setIsOpen] = useState(false);
	const build = workspace.latest_build;
	const { data: templateVersionParameters } = useQuery(
		richParameters(build.template_version_id),
	);
	const { data: buildParameters } = useQuery(
		workspaceBuildParameters(build.id),
	);
	const ephemeralParameters = templateVersionParameters
		? templateVersionParameters.filter((p) => p.ephemeral)
		: undefined;

	return (
		<Popover open={isOpen} onOpenChange={setIsOpen}>
			<PopoverTrigger asChild>
				<TopbarButton
					data-testid="build-parameters-button"
					disabled={disabled}
					className="min-w-fit"
				>
					<ChevronDownIcon />
					<span css={{ ...visuallyHidden }}>{label}</span>
				</TopbarButton>
			</PopoverTrigger>
			<PopoverContent
				align="end"
				className="bg-surface-secondary border-surface-quaternary w-[304px]"
			>
				<BuildParametersPopoverContent
					workspace={workspace}
					ephemeralParameters={ephemeralParameters}
					buildParameters={buildParameters}
					onSubmit={onSubmit}
					setIsOpen={setIsOpen}
				/>
			</PopoverContent>
		</Popover>
	);
};

interface BuildParametersPopoverContentProps {
	workspace: Workspace;
	ephemeralParameters?: TemplateVersionParameter[];
	buildParameters?: WorkspaceBuildParameter[];
	onSubmit: (buildParameters: WorkspaceBuildParameter[]) => void;
	setIsOpen: (newOpen: boolean) => void;
}

const BuildParametersPopoverContent: FC<BuildParametersPopoverContentProps> = ({
	workspace,
	ephemeralParameters,
	buildParameters,
	onSubmit,
	setIsOpen,
}) => {
	const theme = useTheme();

	if (
		!workspace.template_use_classic_parameter_flow &&
		ephemeralParameters &&
		ephemeralParameters.length > 0
	) {
		return (
			<div className="flex flex-col gap-4 p-5">
				<p className="m-0 text-sm text-content-secondary">
					This workspace has ephemeral parameters which may use a temporary
					value on workspace start. Configure the following parameters in
					workspace settings.
				</p>

				<div>
					<ul className="list-none pl-3 space-y-2">
						{ephemeralParameters.map((param) => (
							<li key={param.name}>
								<p className="text-content-primary m-0 font-bold">
									{param.display_name || param.name}
								</p>
								{param.description && (
									<p className="m-0 text-sm text-content-secondary">
										{param.description}
									</p>
								)}
							</li>
						))}
					</ul>
				</div>

				<Link
					href={`/@${workspace.owner_name}/${workspace.name}/settings/parameters`}
					className="self-start"
				>
					Go to workspace parameters
				</Link>
			</div>
		);
	}

	return (
		<>
			{buildParameters && ephemeralParameters ? (
				ephemeralParameters.length > 0 ? (
					<>
						<div
							css={{
								color: theme.palette.text.secondary,
								padding: 20,
								borderBottom: `1px solid ${theme.palette.divider}`,
							}}
						>
							<HelpTooltipTitle>Build options</HelpTooltipTitle>
							<HelpTooltipText>
								These parameters only apply for a single workspace start.
							</HelpTooltipText>
						</div>
						<div css={{ padding: 20 }}>
							<Form
								onSubmit={(buildParameters) => {
									onSubmit(buildParameters);
									setIsOpen(false);
								}}
								ephemeralParameters={ephemeralParameters}
								buildParameters={buildParameters.map(
									(p): AutofillBuildParameter => ({
										...p,
										source: "active_build",
									}),
								)}
							/>
						</div>
					</>
				) : (
					<div
						css={{
							color: theme.palette.text.secondary,
							padding: 20,
							borderBottom: `1px solid ${theme.palette.divider}`,
						}}
					>
						<HelpTooltipTitle>Build options</HelpTooltipTitle>
						<HelpTooltipText>
							This template has no ephemeral build options.
						</HelpTooltipText>
						<HelpTooltipLinksGroup>
							<HelpTooltipLink
								href={docs(
									"/admin/templates/extending-templates/parameters#ephemeral-parameters",
								)}
							>
								Read the docs
							</HelpTooltipLink>
						</HelpTooltipLinksGroup>
					</div>
				)
			) : (
				<Loader />
			)}
		</>
	);
};

interface FormProps {
	ephemeralParameters: TemplateVersionParameter[];
	buildParameters: AutofillBuildParameter[];
	onSubmit: (buildParameters: WorkspaceBuildParameter[]) => void;
}

const Form: FC<FormProps> = ({
	ephemeralParameters,
	buildParameters,
	onSubmit,
}) => {
	const form = useFormik({
		initialValues: {
			rich_parameter_values: getInitialRichParameterValues(
				ephemeralParameters,
				buildParameters,
			),
		},
		onSubmit: (values) => {
			onSubmit(values.rich_parameter_values);
		},
	});
	const getFieldHelpers = getFormHelpers(form);

	return (
		<form onSubmit={form.handleSubmit} data-testid="build-parameters-form">
			<FormFields>
				{ephemeralParameters.map((parameter, index) => {
					return (
						<RichParameterInput
							{...getFieldHelpers(`rich_parameter_values[${index}].value`)}
							key={parameter.name}
							parameter={parameter}
							size="small"
							onChange={async (value) => {
								await form.setFieldValue(`rich_parameter_values[${index}]`, {
									name: parameter.name,
									value: value,
								});
							}}
						/>
					);
				})}
			</FormFields>
			<div css={{ paddingTop: "24px", paddingBottom: "8px" }}>
				<Button
					data-testid="build-parameters-submit"
					type="submit"
					css={{ width: "100%" }}
				>
					Build workspace
				</Button>
			</div>
		</form>
	);
};
