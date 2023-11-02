import { css } from "@emotion/css";
import { type Interpolation, type Theme, useTheme } from "@emotion/react";
import IconButton from "@mui/material/IconButton";
import { EditSquare } from "components/Icons/EditSquare";
import { type FC } from "react";
import { Stack } from "components/Stack/Stack";
import Checkbox from "@mui/material/Checkbox";
import UserIcon from "@mui/icons-material/PersonOutline";
import { Role } from "api/typesGenerated";
import {
  HelpTooltip,
  HelpTooltipText,
  HelpTooltipTitle,
} from "components/HelpTooltip/HelpTooltip";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "components/Popover/Popover";

const roleDescriptions: Record<string, string> = {
  owner:
    "Owner can manage all resources, including users, groups, templates, and workspaces.",
  "user-admin": "User admin can manage all users and groups.",
  "template-admin": "Template admin can manage all templates and workspaces.",
  auditor: "Auditor can access the audit logs.",
  member:
    "Everybody is a member. This is a shared and default role for all users.",
};

const Option: React.FC<{
  value: string;
  name: string;
  description: string;
  isChecked: boolean;
  onChange: (roleName: string) => void;
}> = ({ value, name, description, isChecked, onChange }) => {
  return (
    <label htmlFor={name} css={styles.option}>
      <Stack direction="row" alignItems="flex-start">
        <Checkbox
          id={name}
          size="small"
          css={styles.checkbox}
          value={value}
          checked={isChecked}
          onChange={(e) => {
            onChange(e.currentTarget.value);
          }}
        />
        <Stack spacing={0}>
          <strong>{name}</strong>
          <span css={styles.optionDescription}>{description}</span>
        </Stack>
      </Stack>
    </label>
  );
};

export interface EditRolesButtonProps {
  isLoading: boolean;
  roles: Role[];
  selectedRoleNames: Set<string>;
  onChange: (roles: Role["name"][]) => void;
  isDefaultOpen?: boolean;
  oidcRoleSync: boolean;
  userLoginType: string;
}

export const EditRolesButton: FC<EditRolesButtonProps> = ({
  roles,
  selectedRoleNames,
  onChange,
  isLoading,
  isDefaultOpen = false,
  userLoginType,
  oidcRoleSync,
}) => {
  const theme = useTheme();

  const handleChange = (roleName: string) => {
    if (selectedRoleNames.has(roleName)) {
      const serialized = [...selectedRoleNames];
      onChange(serialized.filter((role) => role !== roleName));
      return;
    }

    onChange([...selectedRoleNames, roleName]);
  };

  const canSetRoles =
    userLoginType !== "oidc" || (userLoginType === "oidc" && !oidcRoleSync);

  if (!canSetRoles) {
    return (
      <HelpTooltip size="small">
        <HelpTooltipTitle>Externally controlled</HelpTooltipTitle>
        <HelpTooltipText>
          Roles for this user are controlled by the OIDC identity provider.
        </HelpTooltipText>
      </HelpTooltip>
    );
  }

  return (
    <Popover isDefaultOpen={isDefaultOpen}>
      <PopoverTrigger>
        <IconButton
          size="small"
          css={styles.editButton}
          title="Edit user roles"
        >
          <EditSquare />
        </IconButton>
      </PopoverTrigger>

      <PopoverContent
        classes={{
          paper: css`
            width: ${theme.spacing(45)};
            margin-top: ${theme.spacing(1)};
            background: ${theme.palette.background.paperLight};
          `,
        }}
      >
        <fieldset
          css={styles.fieldset}
          disabled={isLoading}
          title="Available roles"
        >
          <Stack css={styles.options} spacing={3}>
            {roles.map((role) => (
              <Option
                key={role.name}
                onChange={handleChange}
                isChecked={selectedRoleNames.has(role.name)}
                value={role.name}
                name={role.display_name}
                description={roleDescriptions[role.name] ?? ""}
              />
            ))}
          </Stack>
        </fieldset>
        <div css={styles.footer}>
          <Stack direction="row" alignItems="flex-start">
            <UserIcon css={styles.userIcon} />
            <Stack spacing={0}>
              <strong>Member</strong>
              <span css={styles.optionDescription}>
                {roleDescriptions.member}
              </span>
            </Stack>
          </Stack>
        </div>
      </PopoverContent>
    </Popover>
  );
};

const styles = {
  editButton: (theme) => ({
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
  }),
  fieldset: {
    border: 0,
    margin: 0,
    padding: 0,

    "&:disabled": {
      opacity: 0.5,
    },
  },
  options: (theme) => ({
    padding: theme.spacing(3),
  }),
  option: {
    cursor: "pointer",
    fontSize: 14,
  },
  checkbox: (theme) => ({
    padding: 0,
    position: "relative",
    top: 1, // Alignment

    "& svg": {
      width: theme.spacing(2.5),
      height: theme.spacing(2.5),
    },
  }),
  optionDescription: (theme) => ({
    fontSize: 13,
    color: theme.palette.text.secondary,
    lineHeight: "160%",
  }),
  footer: (theme) => ({
    padding: theme.spacing(3),
    backgroundColor: theme.palette.background.paper,
    borderTop: `1px solid ${theme.palette.divider}`,
    fontSize: 14,
  }),
  userIcon: (theme) => ({
    width: theme.spacing(2.5), // Same as the checkbox
    height: theme.spacing(2.5),
    color: theme.palette.primary.main,
  }),
} satisfies Record<string, Interpolation<Theme>>;
