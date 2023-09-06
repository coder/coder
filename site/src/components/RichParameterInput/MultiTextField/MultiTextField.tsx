import Chip from "@mui/material/Chip"
import FormHelperText from "@mui/material/FormHelperText"
import { makeStyles } from "@mui/styles"
import { FC } from "react"

export type MultiTextFieldProps = {
  label: string
  id?: string
  values: string[]
  onChange: (values: string[]) => void
}

export const MultiTextField: FC<MultiTextFieldProps> = ({
  label,
  id,
  values,
  onChange,
}) => {
  const styles = useStyles()

  return (
    <div>
      <label className={styles.root}>
        {values.map((value, index) => (
          <Chip
            key={index}
            label={value}
            size="small"
            onDelete={() => {
              onChange(values.filter((oldValue) => oldValue !== value))
            }}
          />
        ))}
        <input
          id={id}
          aria-label={label}
          className={styles.input}
          onKeyDown={(event) => {
            if (event.key === ",") {
              event.preventDefault()
              const newValue = event.currentTarget.value
              onChange([...values, newValue])
              event.currentTarget.value = ""
              return
            }

            if (event.key === "Backspace" && event.currentTarget.value === "") {
              event.preventDefault()

              if (values.length === 0) {
                return
              }

              const lastValue = values[values.length - 1]
              onChange(values.slice(0, -1))
              event.currentTarget.value = lastValue
            }
          }}
          onBlur={(event) => {
            if (event.currentTarget.value !== "") {
              const newValue = event.currentTarget.value
              onChange([...values, newValue])
              event.currentTarget.value = ""
            }
          }}
        />
      </label>

      <FormHelperText>{'Type "," to separate the values'}</FormHelperText>
    </div>
  )
}

const useStyles = makeStyles((theme) => ({
  root: {
    border: `1px solid ${theme.palette.divider}`,
    borderRadius: theme.shape.borderRadius,
    minHeight: theme.spacing(6), // Chip height + paddings
    padding: theme.spacing(1.25, 1.75),
    fontSize: theme.spacing(2),
    display: "flex",
    flexWrap: "wrap",
    gap: theme.spacing(1),
    position: "relative",
    margin: theme.spacing(1, 0, 0.5), // Have same margin than TextField

    "&:has(input:focus)": {
      borderColor: theme.palette.primary.main,
      borderWidth: 2,
      // Compensate for the border width
      top: -1,
      left: -1,
    },
  },

  input: {
    flexGrow: 1,
    fontSize: "inherit",
    padding: 0,
    border: "none",
    background: "none",

    "&:focus": {
      outline: "none",
    },
  },
}))
