import ExpandMoreOutlined from "@mui/icons-material/ExpandMoreOutlined";
import Box from "@mui/material/Box";
import Button from "@mui/material/Button";
import { useQuery } from "react-query";
import { getWorkspaceParameters } from "api/api";
import {
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
import { getInitialRichParameterValues } from "utils/richParameters";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
  usePopover,
} from "components/Popover/Popover";

export const BuildParametersPopover = ({
  workspace,
  disabled,
  onSubmit,
}: {
  workspace: Workspace;
  disabled?: boolean;
  onSubmit: (buildParameters: WorkspaceBuildParameter[]) => void;
}) => {
  return (
    <Popover>
      <PopoverTrigger>
        <Button
          data-testid="build-parameters-button"
          disabled={disabled}
          color="neutral"
          sx={{ px: 0 }}
        >
          <ExpandMoreOutlined sx={{ fontSize: 16 }} />
        </Button>
      </PopoverTrigger>
      <PopoverContent
        horizontal="right"
        css={(theme) => ({ ".MuiPaper-root": { width: theme.spacing(38) } })}
      >
        <BuildParametersPopoverContent
          workspace={workspace}
          onSubmit={onSubmit}
        />
      </PopoverContent>
    </Popover>
  );
};

const BuildParametersPopoverContent = ({
  workspace,
  onSubmit,
}: {
  workspace: Workspace;
  onSubmit: (buildParameters: WorkspaceBuildParameter[]) => void;
}) => {
  const popover = usePopover();
  const { data: parameters } = useQuery({
    queryKey: ["workspace", workspace.id, "parameters"],
    queryFn: () => getWorkspaceParameters(workspace),
    enabled: popover.isOpen,
  });
  const ephemeralParameters = parameters
    ? parameters.templateVersionRichParameters.filter((p) => p.ephemeral)
    : undefined;

  return (
    <>
      {parameters && parameters.buildParameters && ephemeralParameters ? (
        ephemeralParameters.length > 0 ? (
          <>
            <Box
              sx={{
                color: (theme) => theme.palette.text.secondary,
                p: 2.5,
                borderBottom: (theme) => `1px solid ${theme.palette.divider}`,
              }}
            >
              <HelpTooltipTitle>Build Options</HelpTooltipTitle>
              <HelpTooltipText>
                These parameters only apply for a single workspace start.
              </HelpTooltipText>
            </Box>
            <Box sx={{ p: 2.5 }}>
              <Form
                onSubmit={(buildParameters) => {
                  onSubmit(buildParameters);
                  popover.setIsOpen(false);
                }}
                ephemeralParameters={ephemeralParameters}
                buildParameters={parameters.buildParameters}
              />
            </Box>
          </>
        ) : (
          <Box
            sx={{
              color: (theme) => theme.palette.text.secondary,
              p: 2.5,
              borderBottom: (theme) => `1px solid ${theme.palette.divider}`,
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
          </Box>
        )
      ) : (
        <Loader />
      )}
    </>
  );
};

const Form = ({
  ephemeralParameters,
  buildParameters,
  onSubmit,
}: {
  ephemeralParameters: TemplateVersionParameter[];
  buildParameters: WorkspaceBuildParameter[];
  onSubmit: (buildParameters: WorkspaceBuildParameter[]) => void;
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
      <Box sx={{ py: 3, pb: 1 }}>
        <Button
          data-testid="build-parameters-submit"
          type="submit"
          variant="contained"
          color="primary"
          sx={{ width: "100%" }}
        >
          Build workspace
        </Button>
      </Box>
    </form>
  );
};
