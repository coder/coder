import FormControlLabel from "@mui/material/FormControlLabel";
import Radio from "@mui/material/Radio";
import RadioGroup from "@mui/material/RadioGroup";
import { API } from "api/api";
import type { Template, TemplateVersionParameter } from "api/typesGenerated";
import { Button } from "components/Button/Button";
import { FormSection, VerticalForm } from "components/Form/Form";
import { Input } from "components/Input/Input";
import { Label } from "components/Label/Label";
import { Loader } from "components/Loader/Loader";
import { RichParameterInput } from "components/RichParameterInput/RichParameterInput";
import { useDebouncedFunction } from "hooks/debounce";
import { useClipboard } from "hooks/useClipboard";
import { CheckIcon, CopyIcon } from "lucide-react";
import { useTemplateLayoutContext } from "pages/TemplatePage/TemplateLayout";
import { type FC, useEffect, useId, useState } from "react";
import { useQuery } from "react-query";
import { nameValidator } from "utils/formUtils";
import { pageTitle } from "utils/page";
import { getInitialRichParameterValues } from "utils/richParameters";
import { paramsUsedToCreateWorkspace } from "utils/workspace";
import { ValidationError } from "yup";

type ButtonValues = Record<string, string>;

const TemplateEmbedPage: FC = () => {
	const { template } = useTemplateLayoutContext();
	const { data: templateParameters } = useQuery({
		queryKey: ["template", template.id, "embed"],
		queryFn: () =>
			API.getTemplateVersionRichParameters(template.active_version_id),
	});

	return (
		<>
			<title>{pageTitle(template.name)}</title>

			<TemplateEmbedPageView
				template={template}
				templateParameters={templateParameters?.filter(
					paramsUsedToCreateWorkspace,
				)}
			/>
		</>
	);
};

interface TemplateEmbedPageViewProps {
	template: Template;
	templateParameters?: TemplateVersionParameter[];
}

const deploymentUrl = `${window.location.protocol}//${window.location.host}`;

function getClipboardCopyContent(
	templateName: string,
	organization: string,
	buttonValues: ButtonValues | undefined,
): string {
	const createWorkspaceUrl = `${deploymentUrl}/templates/${organization}/${templateName}/workspace`;
	const createWorkspaceParams = new URLSearchParams(buttonValues);
	if (createWorkspaceParams.get("name") === "") {
		createWorkspaceParams.delete("name"); // no default workspace name if empty
	}
	const buttonUrl = `${createWorkspaceUrl}?${createWorkspaceParams.toString()}`;

	return `[![Open in Coder](${deploymentUrl}/open-in-coder.svg)](${buttonUrl})`;
}

const workspaceNameValidator = nameValidator("Workspace name");

export const TemplateEmbedPageView: FC<TemplateEmbedPageViewProps> = ({
	template,
	templateParameters,
}) => {
	const [buttonValues, setButtonValues] = useState<ButtonValues | undefined>();
	const clipboard = useClipboard();

	// template parameters is async so we need to initialize the values after it
	// is loaded
	useEffect(() => {
		if (templateParameters && !buttonValues) {
			const buttonValues: ButtonValues = {
				mode: "manual",
				name: "",
			};
			for (const parameter of getInitialRichParameterValues(
				templateParameters,
			)) {
				buttonValues[`param.${parameter.name}`] = parameter.value;
			}
			setButtonValues(buttonValues);
		}
	}, [buttonValues, templateParameters]);

	const [workspaceNameError, setWorkspaceNameError] = useState("");
	const validateWorkspaceName = (workspaceName: string) => {
		try {
			if (workspaceName) {
				workspaceNameValidator.validateSync(workspaceName);
			}
			setWorkspaceNameError("");
		} catch (e) {
			if (e instanceof ValidationError) {
				setWorkspaceNameError(e.message);
			}
		}
	};
	const { debounced: debouncedValidateWorkspaceName } = useDebouncedFunction(
		validateWorkspaceName,
		500,
	);

	const hookId = useId();
	const defaultWorkspaceNameID = `${hookId}-default-workspace-name`;

	return (
		<>
			<title>{pageTitle(template.name)}</title>

			{!buttonValues || !templateParameters ? (
				<Loader />
			) : (
				<div className="flex items-start gap-12">
					<div className="max-w-3xl">
						<VerticalForm>
							<FormSection
								title="Creation mode"
								description="By changing the mode to automatic, when the user clicks the button, the workspace will be created automatically instead of showing a form to the user."
							>
								<RadioGroup
									defaultValue={buttonValues.mode}
									onChange={(_, v) => {
										setButtonValues((buttonValues) => ({
											...buttonValues,
											mode: v,
										}));
									}}
								>
									<FormControlLabel
										value="manual"
										control={<Radio size="small" />}
										label="Manual"
									/>
									<FormControlLabel
										value="auto"
										control={<Radio size="small" />}
										label="Automatic"
									/>
								</RadioGroup>
							</FormSection>

							<div className="flex flex-col gap-1">
								<Label className="text-md" htmlFor={defaultWorkspaceNameID}>
									Workspace name
								</Label>
								<div className="text-sm text-content-secondary pb-3">
									Default name for the new workspace
								</div>
								<Input
									id={defaultWorkspaceNameID}
									value={buttonValues.name}
									onChange={(event) => {
										debouncedValidateWorkspaceName(event.target.value);
										setButtonValues((buttonValues) => ({
											...buttonValues,
											name: event.target.value,
										}));
									}}
								/>
								<div className="text-sm text-highlight-red mt-1" role="alert">
									{workspaceNameError}
								</div>
							</div>

							{templateParameters.length > 0 && (
								<div
									css={{ display: "flex", flexDirection: "column", gap: 36 }}
								>
									{templateParameters.map((parameter) => {
										const parameterValue =
											buttonValues[`param.${parameter.name}`] ?? "";

										return (
											<RichParameterInput
												value={parameterValue}
												onChange={async (value) => {
													setButtonValues((buttonValues) => ({
														...buttonValues,
														[`param.${parameter.name}`]: value,
													}));
												}}
												key={parameter.name}
												parameter={parameter}
											/>
										);
									})}
								</div>
							)}
						</VerticalForm>
					</div>

					<div
						css={(theme) => ({
							// 80px for padding, 36px is for the status bar. We want to use `vh`
							// so that it will be relative to the screen and not the parent layout.
							height: "calc(100vh - (80px + 36px))",
							top: 40,
							position: "sticky",
							display: "flex",
							padding: 64,
							flex: 1,
							alignItems: "center",
							justifyContent: "center",
							borderRadius: 8,
							backgroundColor: theme.palette.background.paper,
							border: `1px solid ${theme.palette.divider}`,
						})}
					>
						<img src="/open-in-coder.svg" alt="Open in Coder button" />
						<div
							css={{
								padding: "48px 16px",
								position: "absolute",
								bottom: 0,
								left: 0,
								display: "flex",
								justifyContent: "center",
								width: "100%",
							}}
						>
							<Button
								className="rounded-full"
								disabled={clipboard.showCopiedSuccess}
								onClick={() => {
									const textToCopy = getClipboardCopyContent(
										template.name,
										template.organization_name,
										buttonValues,
									);
									clipboard.copyToClipboard(textToCopy);
								}}
							>
								{clipboard.showCopiedSuccess ? <CheckIcon /> : <CopyIcon />}
								Copy button code
							</Button>
						</div>
					</div>
				</div>
			)}
		</>
	);
};

export default TemplateEmbedPage;
