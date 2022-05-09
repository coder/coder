import Checkbox from "@material-ui/core/Checkbox"
import MenuItem from "@material-ui/core/MenuItem"
import Select from "@material-ui/core/Select"
import makeStyles from "@material-ui/styles/makeStyles"
import React from "react"
import { Role } from "../../api/typesGenerated"

export interface RoleSelectProps {
  roles: Role[]
  selectedRoles: Role[]
  onChange: (roles: Role["name"][]) => void
  loading?: boolean
}

export const RoleSelect: React.FC<RoleSelectProps> = ({ roles, selectedRoles, loading, onChange }) => {
  const styles = useStyles()
  const value = selectedRoles.map((r) => r.name)
  const renderValue = () => selectedRoles.map((r) => r.display_name).join(", ")

  return (
    <Select
      multiple
      value={value}
      renderValue={renderValue}
      variant="outlined"
      className={styles.select}
      onChange={(e) => {
        const { value } = e.currentTarget
        onChange(value as string[])
      }}
    >
      {roles.map((r) => {
        const isChecked = selectedRoles.some((selectedRole) => selectedRole.name === r.name)

        return (
          <MenuItem key={r.name} value={r.name} disabled={loading}>
            <Checkbox color="primary" checked={isChecked} /> {r.display_name}
          </MenuItem>
        )
      })}
    </Select>
  )
}

const useStyles = makeStyles(() => ({
  select: {
    margin: 0,
  },
}))
