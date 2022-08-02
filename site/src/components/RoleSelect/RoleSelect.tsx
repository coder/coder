import Checkbox from "@material-ui/core/Checkbox"
import MenuItem from "@material-ui/core/MenuItem"
import Select from "@material-ui/core/Select"
import { makeStyles, Theme } from "@material-ui/core/styles"
import { FC } from "react"
import { Role } from "../../api/typesGenerated"

export const Language = {
  label: "Roles",
}
export interface RoleSelectProps {
  roles: Role[]
  selectedRoles: Role[]
  onChange: (roles: Role["name"][]) => void
  loading?: boolean
  open?: boolean
}

export const RoleSelect: FC<React.PropsWithChildren<RoleSelectProps>> = ({
  roles,
  selectedRoles,
  loading,
  onChange,
  open,
}) => {
  const styles = useStyles()
  const value = selectedRoles.map((r) => r.name)
  const renderValue = () => selectedRoles.map((r) => r.display_name).join(", ")
  const sortedRoles = roles.sort((a, b) => a.display_name.localeCompare(b.display_name))

  return (
    <Select
      aria-label={Language.label}
      open={open}
      multiple
      value={value}
      renderValue={renderValue}
      variant="outlined"
      className={styles.select}
      onChange={(e) => {
        const { value } = e.target
        onChange(value as string[])
      }}
    >
      {sortedRoles.map((r) => {
        const isChecked = selectedRoles.some((selectedRole) => selectedRole.name === r.name)

        return (
          <MenuItem key={r.name} value={r.name} disabled={loading}>
            <Checkbox size="small" color="primary" checked={isChecked} /> {r.display_name}
          </MenuItem>
        )
      })}
    </Select>
  )
}

const useStyles = makeStyles((theme: Theme) => ({
  select: {
    margin: 0,
    // Set a fixed width for the select. It avoids selects having different sizes
    // depending on how many roles they have selected.
    width: theme.spacing(25),
    "& .MuiSelect-root": {
      // Adjusting padding because it does not have label
      paddingTop: theme.spacing(1.5),
      paddingBottom: theme.spacing(1.5),
    },
  },
}))
