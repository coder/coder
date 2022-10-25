import Box from "@material-ui/core/Box"
import makeStyles from "@material-ui/core/styles/makeStyles"
import TextField from "@material-ui/core/TextField"
import Typography from "@material-ui/core/Typography"
import WarningIcon from "@material-ui/icons/Lock"
import ToggleButton from "@material-ui/lab/ToggleButton"
import ToggleButtonGroup from "@material-ui/lab/ToggleButtonGroup"
import {
  TemplateVersionParameter,
  WorkspaceBuildParameter,
} from "api/typesGenerated"
import React, { FC, useEffect, useState } from "react"

export interface WorkspaceParameterProps {
  disabled?: boolean
  onChange?: (value: string) => void

  templateParameter: TemplateVersionParameter
  workspaceBuildParameter?: WorkspaceBuildParameter
}

export const WorkspaceParameter: FC<
  React.PropsWithChildren<WorkspaceParameterProps>
> = ({ templateParameter, workspaceBuildParameter, disabled, onChange }) => {
  const [value, setValue] = useState(
    workspaceBuildParameter?.value || templateParameter.default_value,
  )
  const [error, setError] = useState<string>()
  const styles = useStyles()
  const hasOptions = templateParameter.options.length > 0
  useEffect(() => {
    if (onChange) {
      onChange(value)
    }
  }, [onChange, value])

  return (
    <div className={styles.root}>
      <Box marginBottom="12px">
        <Box display="flex" alignItems="center" marginBottom="8px">
          {templateParameter.icon !== "" && (
            <img
              className={styles.icon}
              alt={`${templateParameter.name} icon`}
              src={templateParameter.icon}
            />
          )}
          <h1 className={styles.name}>{templateParameter.name}</h1>
          {!templateParameter.mutable && (
            <div className={styles.immutable}>
              <WarningIcon className="icon" />
              Cannot be changed after create.
            </div>
          )}
        </Box>
        {templateParameter.description && (
          <Typography
            variant="body1"
            color="textSecondary"
            className={styles.description}
          >
            {templateParameter.description}
          </Typography>
        )}
      </Box>
      {hasOptions && (
        <ToggleButtonGroup
          className={styles.options}
          value={value}
          exclusive
          onChange={(_, selected) => setValue(selected)}
        >
          {templateParameter.options.map((option) => (
            <ToggleButton
              key={option.name}
              className={styles.option}
              value={option.value}
              disabled={disabled}
            >
              <Box display="flex" flexDirection="column">
                <Box display="flex" alignItems="center" justifyContent="center">
                  {option.icon && (
                    <img
                      alt={`${option.name} icon`}
                      src={option.icon}
                      className="icon"
                    />
                  )}
                  <Typography variant="h6" className="title">
                    {option.name}
                  </Typography>
                </Box>
                {option.description && (
                  <Typography variant="body2" className="description">
                    {option.description}
                  </Typography>
                )}
              </Box>
            </ToggleButton>
          ))}
        </ToggleButtonGroup>
      )}
      {!hasOptions && templateParameter.type === "string" && (
        <TextField
          id={templateParameter.name}
          defaultValue={workspaceBuildParameter?.value}
          placeholder={templateParameter.default_value}
          type="text"
          onChange={(event) => {
            setValue(event.target.value)
          }}
          inputProps={{
            pattern: templateParameter.validation_regex,
          }}
          onBlur={(event) => {
            if (event.target.checkValidity()) {
              setError(undefined)
              return
            }
            setError(templateParameter.validation_error)
          }}
          error={Boolean(error)}
          helperText={error}
          fullWidth
          disabled={disabled}
        />
      )}
      {!hasOptions && templateParameter.type === "number" && (
        <TextField
          id={templateParameter.name}
          defaultValue={workspaceBuildParameter?.value}
          placeholder={templateParameter.default_value}
          type="number"
          inputProps={{
            min: 1,
            max: 500,
          }}
          onChange={(event) => {
            setValue(event.target.value)
            if (
              parseInt(event.target.value) < templateParameter.validation_min
            ) {
              return setError(`Must be >= ${templateParameter.validation_min}.`)
            }
            if (
              parseInt(event.target.value) > templateParameter.validation_max
            ) {
              return setError(`Must be <= ${templateParameter.validation_max}.`)
            }
            setError(undefined)
          }}
          error={Boolean(error)}
          helperText={
            error ||
            (templateParameter.validation_min !== 0 &&
              templateParameter.validation_max !== 0)
              ? `Must be between ${templateParameter.validation_min} and ${templateParameter.validation_max}.`
              : undefined
          }
          fullWidth
          disabled
        />
      )}
    </div>
  )
}

const useStyles = makeStyles((theme) => ({
  root: {
    maxWidth: 900,
  },
  name: {
    fontSize: 20,
    margin: 0,
  },
  description: {
    marginBottom: theme.spacing(2),
  },
  icon: {
    width: 28,
    height: 28,
    marginRight: theme.spacing(1),
  },
  immutable: {
    marginLeft: theme.spacing(3),
    color: theme.palette.text.hint,
    display: "flex",
    alignItems: "center",

    "& .icon": {
      width: 14,
      height: 14,
      marginRight: 4,
    },
  },
  options: {
    display: "grid",
    gridTemplateColumns: "repeat(auto-fit, minmax(min(100%/3.5), 1fr))",
    gap: 16,
    width: "100%",
    background: "transparent",

    [theme.breakpoints.down("xs")]: {
      gridTemplateColumns: "1fr",
    },
  },
  option: {
    background: theme.palette.background.paper,
    textTransform: "unset",
    letterSpacing: "unset",
    padding: "6px 12px",
    borderWidth: "2px",
    height: "unset",
    minHeight: theme.spacing(6),
    flex: 1,
    transition: "border 250ms ease",

    "&:not(:first-of-type)": {
      borderRadius: theme.shape.borderRadius,
      borderLeft: "2px solid rgba(255, 255, 255, 0.12)",
      marginLeft: "unset",
    },
    "&:first-of-type": {
      borderRadius: theme.shape.borderRadius,
    },

    "&.Mui-selected": {
      borderColor: `${theme.palette.primary.light}`,
    },

    "& .description": {
      fontSize: 14,
      marginTop: theme.spacing(1),
    },

    "&.Mui-selected .description": {
      color: theme.palette.text.secondary,
    },

    "& .title": {
      fontWeight: "500",
      color: theme.palette.text.hint,
    },

    "&.Mui-selected .title": {
      color: theme.palette.text.primary,
    },

    "& .icon": {
      width: 20,
      height: 20,
      marginRight: theme.spacing(1.5),
      transition: "filter 250ms ease",
    },
  },
}))
