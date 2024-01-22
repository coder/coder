import ExpandMoreOutlined from "@mui/icons-material/ExpandMoreOutlined";
import Button from "@mui/material/Button";
import { useTheme } from "@emotion/react";
import { type FC } from "react";
import { useQuery } from "react-query";
import { getWorkspaceParameters } from "api/api";
import type {
  TemplateVersionParameter,
  Workspace,
  WorkspaceBuildParameter,
} from "api/typesGenerated";
import { FormFields } from "components/Form/Form";
import { Loader } from "components/Loader/Loader";
import { RichParameterInput } from "components/RichParameterInput/RichParameterInput";
import {
  HelpTooltipLink,
  HelpTooltipLinksGroup,
  HelpTooltipText,
  HelpTooltipTitle,
} from "components/HelpTooltip/HelpTooltip";
import { useFormik } from "formik";
import { docs } from "utils/docs";
import { getFormHelpers } from "utils/formUtils";
import {
  AutofillBuildParameter,
  getInitialRichParameterValues,
} from "utils/richParameters";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
  usePopover,
} from "components/Popover/Popover";
import { TopbarButton } from "components/FullPageLayout/Topbar";

interface BuildParametersPopoverProps {
  workspace: Workspace;
  disabled?: boolean;
  onSubmit: (buildParameters: WorkspaceBuildParameter[]) => void;
}

export const BuildParametersPopover: FC<BuildParametersPopoverProps> = ({
  workspace,
  disabled,
  onSubmit,
}) => {
  const { data: parameters } = useQuery({
    queryKey: ["workspace", workspace.id, "parameters"],
    queryFn: () => getWorkspaceParameters(workspace),
  });
  const ephemeralParameters = parameters
    ? parameters.templateVersionRichParameters.filter((p) => p.ephemeral)
    : undefined;

  return (
    <Popover>
      <PopoverTrigger>
        <TopbarButton
          data-testid="build-parameters-button"
          disabled={disabled}
          color="neutral"
          css={{ paddingLeft: 0, paddingRight: 0, minWidth: "28px !important" }}
        >
          <ExpandMoreOutlined css={{ fontSize: 14 }} />
        </TopbarButton>
      </PopoverTrigger>
      <PopoverContent
        horizontal="right"
        css={{ ".MuiPaper-root": { width: 304 } }}
      >
        <BuildParametersPopoverContent
          ephemeralParameters={ephemeralParameters}
          buildParameters={parameters?.buildParameters}
          onSubmit={onSubmit}
        />
      </PopoverContent>
    </Popover>
  );
};

interface BuildParametersPopoverContentProps {
  ephemeralParameters?: TemplateVersionParameter[];
  buildParameters?: WorkspaceBuildParameter[];
  onSubmit: (buildParameters: WorkspaceBuildParameter[]) => void;
}

const BuildParametersPopoverContent: FC<BuildParametersPopoverContentProps> = ({
  ephemeralParameters,
  buildParameters,
  onSubmit,
}) => {
  const theme = useTheme();
  const popover = usePopover();

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
              <HelpTooltipTitle>Build Options</HelpTooltipTitle>
              <HelpTooltipText>
                These parameters only apply for a single workspace start.
              </HelpTooltipText>
            </div>
            <div css={{ padding: 20 }}>
              <Form
                onSubmit={(buildParameters) => {
                  onSubmit(buildParameters);
                  popover.setIsOpen(false);
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
            <HelpTooltipTitle>Build Options</HelpTooltipTitle>
            <HelpTooltipText>
              This template has no ephemeral build options.
            </HelpTooltipText>
            <HelpTooltipLinksGroup>
              <HelpTooltipLink
                href={docs("/templates/parameters#ephemeral-parameters")}
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
              {...getFieldHelpers("rich_parameter_values[" + index + "].value")}
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
          variant="contained"
          color="primary"
          css={{ width: "100%" }}
        >
          Build workspace
        </Button>
      </div>
    </form>
  );
};
