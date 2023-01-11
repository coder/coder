import IconButton from "@material-ui/core/IconButton"
import { EditSquare } from "components/Icons/EditSquare"
import { useRef, useState, FC } from "react"
import { makeStyles } from "@material-ui/core/styles"
import { useTranslation } from "react-i18next"
import Popover from "@material-ui/core/Popover"
import { Stack } from "components/Stack/Stack"
import Checkbox from "@material-ui/core/Checkbox"
import UserIcon from "@material-ui/icons/PersonOutline"
import { Role } from "api/typesGenerated"

const Option: React.FC<{
  value: string
  name: string
  description: string
  isChecked: boolean
  onChange: (roleName: string) => void
}> = ({ value, name, description, isChecked, onChange }) => {
  const styles = useStyles()

  return (
    <label htmlFor={name} className={styles.option}>
      <Stack direction="row" alignItems="flex-start">
        <Checkbox
          id={name}
          size="small"
          color="primary"
          className={styles.checkbox}
          value={value}
          checked={isChecked}
          onChange={(e) => {
            onChange(e.currentTarget.value)
          }}
        />
        <Stack spacing={0.5}>
          <strong>{name}</strong>
          <span className={styles.optionDescription}>{description}</span>
        </Stack>
      </Stack>
    </label>
  )
}

export interface EditRolesButtonProps {
  isLoading: boolean
  roles: Role[]
  selectedRoles: Role[]
  onChange: (roles: Role["name"][]) => void
  defaultIsOpen?: boolean
}

export const EditRolesButton: FC<EditRolesButtonProps> = ({
  roles,
  selectedRoles,
  onChange,
  isLoading,
  defaultIsOpen = false,
}) => {
  const styles = useStyles()
  const { t } = useTranslation("usersPage")
  const anchorRef = useRef<HTMLButtonElement>(null)
  const [isOpen, setIsOpen] = useState(defaultIsOpen)
  const id = isOpen ? "edit-roles-popover" : undefined
  const selectedRoleNames = selectedRoles.map((role) => role.name)

  const handleChange = (roleName: string) => {
    if (selectedRoleNames.includes(roleName)) {
      onChange(selectedRoleNames.filter((role) => role !== roleName))
      return
    }

    onChange([...selectedRoleNames, roleName])
  }

  return (
    <>
      <IconButton
        ref={anchorRef}
        size="small"
        className={styles.editButton}
        title={t("editUserRolesTooltip")}
        onClick={() => setIsOpen(true)}
      >
        <EditSquare />
      </IconButton>

      <Popover
        id={id}
        open={isOpen}
        anchorEl={anchorRef.current}
        onClose={() => setIsOpen(false)}
        anchorOrigin={{
          vertical: "bottom",
          horizontal: "left",
        }}
        transformOrigin={{
          vertical: "top",
          horizontal: "left",
        }}
        classes={{ paper: styles.popoverPaper }}
      >
        <fieldset
          className={styles.fieldset}
          disabled={isLoading}
          title={t("fieldSetRolesTooltip")}
        >
          <Stack className={styles.options} spacing={3}>
            {roles.map((role) => (
              <Option
                key={role.name}
                onChange={handleChange}
                isChecked={selectedRoleNames.includes(role.name)}
                value={role.name}
                name={role.display_name}
                description={t(`roleDescription.${role.name}`)}
              />
            ))}
          </Stack>
        </fieldset>
        <div className={styles.footer}>
          <Stack direction="row" alignItems="flex-start">
            <UserIcon className={styles.userIcon} />
            <Stack spacing={0.5}>
              <strong>{t("member")}</strong>
              <span className={styles.optionDescription}>
                {t("roleDescription.member")}
              </span>
            </Stack>
          </Stack>
        </div>
      </Popover>
    </>
  )
}

const useStyles = makeStyles((theme) => ({
  editButton: {
    color: theme.palette.text.secondary,

    "& .MuiSvgIcon-root": {
      width: theme.spacing(2),
      height: theme.spacing(2),
      position: "relative",
      top: -2, // Align the pencil square
    },

    "&:hover": {
      color: theme.palette.text.primary,
      backgroundColor: "transparent",
    },
  },
  popoverPaper: {
    width: theme.spacing(45),
    marginTop: theme.spacing(1),
    background: theme.palette.background.paperLight,
  },
  fieldset: {
    border: 0,
    margin: 0,
    padding: 0,

    "&:disabled": {
      opacity: 0.5,
    },
  },
  options: {
    padding: theme.spacing(3),
  },
  option: {
    cursor: "pointer",
  },
  checkbox: {
    padding: 0,
    position: "relative",
    top: 1, // Alignment

    "& svg": {
      width: theme.spacing(2.5),
      height: theme.spacing(2.5),
    },
  },
  optionDescription: {
    fontSize: 12,
    color: theme.palette.text.secondary,
  },
  footer: {
    padding: theme.spacing(3),
    backgroundColor: theme.palette.background.paper,
    borderTop: `1px solid ${theme.palette.divider}`,
  },
  userIcon: {
    width: theme.spacing(2.5), // Same as the checkbox
    height: theme.spacing(2.5),
    color: theme.palette.primary.main,
  },
}))
