import Box from "@material-ui/core/Box"
import MenuItem from "@material-ui/core/MenuItem"
import { makeStyles } from "@material-ui/core/styles"
import Typography from "@material-ui/core/Typography"
import React from "react"

import { formTextFieldFactory, FormTextFieldProps } from "./FormTextField"

export interface DropdownItem {
  value: string
  name: string
  description?: string
}

export interface FormDropdownFieldProps<T> extends FormTextFieldProps<T> {
  items: DropdownItem[]
}

export const formDropdownFieldFactory = <T,>(): React.FC<FormDropdownFieldProps<T>> => {
  const FormTextField = formTextFieldFactory<T>()

  const component: React.FC<FormDropdownFieldProps<T>> = ({ items, ...props }) => {
    const styles = useStyles()
    return (
      <FormTextField select {...props}>
        {items.map((item: DropdownItem) => (
          <MenuItem key={item.value} value={item.value}>
            <Box alignItems="center" display="flex">
              <Box ml={1}>
                <Typography>{item.name}</Typography>
              </Box>
              {item.description && (
                <Box ml={1}>
                  <Typography className={styles.hintText} variant="caption">
                    {item.description}
                  </Typography>
                </Box>
              )}
            </Box>
          </MenuItem>
        ))}
      </FormTextField>
    )
  }

  // Required when using an anonymous factory function
  component.displayName = "FormDropdownField"
  return component
}

const useStyles = makeStyles({
  hintText: {
    opacity: 0.75,
  },
})
