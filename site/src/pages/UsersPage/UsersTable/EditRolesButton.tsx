import IconButton from "@mui/material/IconButton"
import { EditSquare } from "components/Icons/EditSquare"
import { useRef, useState, FC } from "react"
import { makeStyles } from "@mui/styles"
import { useTranslation } from "react-i18next"
import Popover from "@mui/material/Popover"
import { Stack } from "components/Stack/Stack"
import Checkbox from "@mui/material/Checkbox"
import UserIcon from "@mui/icons-material/PersonOutline"
import { Role } from "api/typesGenerated"
import {
  HelpTooltip,
  HelpTooltipText,
  HelpTooltipTitle,
} from "components/HelpTooltip/HelpTooltip"
import { Maybe } from "components/Conditionals/Maybe"

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
          className={styles.checkbox}
          value={value}
          checked={isChecked}
          onChange={(e) => {
            onChange(e.currentTarget.value)
          }}
        />
        <Stack spacing={0}>
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
  oidcRoleSync: boolean
  userLoginType: string
}

export const EditRolesButton: FC<EditRolesButtonProps> = ({
  roles,
  selectedRoles,
  onChange,
  isLoading,
  defaultIsOpen = false,
  userLoginType,
  oidcRoleSync,
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

  const canSetRoles =
    userLoginType !== "oidc" || (userLoginType === "oidc" && !oidcRoleSync)

  return (
    <>
      <Maybe condition={canSetRoles}>
        <IconButton
          ref={anchorRef}
          size="small"
          className={styles.editButton}
          title={t("editUserRolesTooltip") || ""}
          onClick={() => setIsOpen(true)}
        >
          <EditSquare />
        </IconButton>
      </Maybe>
      <Maybe condition={!canSetRoles}>
        <HelpTooltip size="small">
          <HelpTooltipTitle>Externally controlled</HelpTooltipTitle>
          <HelpTooltipText>
            Roles for this user are controlled by the OIDC identity provider.
          </HelpTooltipText>
        </HelpTooltip>
      </Maybe>

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
          title={t("fieldSetRolesTooltip") || ""}
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
            <Stack spacing={0}>
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
      opacity: 0,
    },
  },
  options: {
    padding: theme.spacing(3),
  },
  option: {
    cursor: "pointer",
    fontSize: 14,
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
    fontSize: 13,
    color: theme.palette.text.secondary,
    lineHeight: "160%",
  },
  footer: {
    padding: theme.spacing(3),
    backgroundColor: theme.palette.background.paper,
    borderTop: `1px solid ${theme.palette.divider}`,
    fontSize: 14,
  },
  userIcon: {
    width: theme.spacing(2.5), // Same as the checkbox
    height: theme.spacing(2.5),
    color: theme.palette.primary.main,
  },
}))
