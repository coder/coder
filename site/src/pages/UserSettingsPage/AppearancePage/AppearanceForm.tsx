import { visuallyHidden } from "@mui/utils";
import { type Interpolation } from "@emotion/react";
import { type FC, useMemo } from "react";
import type { UpdateUserAppearanceSettingsRequest } from "api/typesGenerated";
import themes, { DEFAULT_THEME, type Theme } from "theme";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { Stack } from "components/Stack/Stack";

export interface AppearanceFormProps {
  isUpdating?: boolean;
  error?: unknown;
  initialValues: UpdateUserAppearanceSettingsRequest;
  onSubmit: (values: UpdateUserAppearanceSettingsRequest) => Promise<unknown>;

  // temporary, so that storybook can test the right thing without showing
  // a semi-broken auto theme to users. will be removed when light mode is done.
  enableAuto?: boolean;
}

export const AppearanceForm: FC<AppearanceFormProps> = ({
  isUpdating,
  error,
  onSubmit,
  initialValues,
  enableAuto,
}) => {
  const currentTheme = initialValues.theme_preference || DEFAULT_THEME;

  const onChangeTheme = async (theme: string) => {
    if (isUpdating) {
      return;
    }

    await onSubmit({ theme_preference: theme });
  };

  return (
    <form>
      {Boolean(error) && <ErrorAlert error={error} />}

      <Stack direction="row" wrap="wrap">
        {enableAuto && (
          <AutoThemePreviewButton
            displayName="Auto"
            active={currentTheme === "auto"}
            themes={[themes.dark, themes.light]}
            onSelect={() => onChangeTheme("auto")}
          />
        )}
        <ThemePreviewButton
          displayName="Dark"
          active={currentTheme === "dark"}
          theme={themes.dark}
          onSelect={() => onChangeTheme("dark")}
        />
        <ThemePreviewButton
          displayName="Dark blue"
          active={currentTheme === "darkBlue"}
          theme={themes.darkBlue}
          onSelect={() => onChangeTheme("darkBlue")}
        />
      </Stack>
    </form>
  );
};

interface AutoThemePreviewButtonProps {
  active?: boolean;
  className?: string;
  displayName: string;
  themes: [Theme, Theme];
  onSelect?: () => void;
}

const AutoThemePreviewButton: FC<AutoThemePreviewButtonProps> = ({
  active,
  className,
  displayName,
  themes,
  onSelect,
}) => {
  const [leftTheme, rightTheme] = themes;

  return (
    <>
      <input
        type="radio"
        name="theme"
        id={displayName}
        value={displayName}
        checked={active}
        onChange={onSelect}
        css={{ ...visuallyHidden }}
      />
      <label htmlFor={displayName} className={className}>
        <ThemePreview
          css={{
            // This half is absolute to not advance the layout (which would offset the second half)
            position: "absolute",
            // Slightly past the bounding box to avoid cutting off the outline
            clipPath: "polygon(-5% -5%, 50% -5%, 50% 105%, -5% 105%)",
          }}
          active={active}
          displayName={displayName}
          theme={leftTheme}
        />
        <ThemePreview
          active={active}
          displayName={displayName}
          theme={rightTheme}
        />
      </label>
    </>
  );
};

interface ThemePreviewButtonProps {
  active?: boolean;
  className?: string;
  displayName: string;
  theme: Theme;
  onSelect?: () => void;
}

const ThemePreviewButton: FC<ThemePreviewButtonProps> = ({
  active,
  className,
  displayName,
  theme,
  onSelect,
}) => {
  return (
    <>
      <input
        type="radio"
        name="theme"
        id={displayName}
        value={displayName}
        checked={active}
        onChange={onSelect}
        css={{ ...visuallyHidden }}
      />
      <label htmlFor={displayName} className={className}>
        <ThemePreview active={active} displayName={displayName} theme={theme} />
      </label>
    </>
  );
};

interface ThemePreviewProps {
  active?: boolean;
  className?: string;
  displayName: string;
  theme: Theme;
}

const ThemePreview: FC<ThemePreviewProps> = ({
  active,
  className,
  displayName,
  theme,
}) => {
  const styles = useMemo(
    () =>
      ({
        container: {
          backgroundColor: theme.palette.background.default,
          border: `1px solid ${theme.palette.divider}`,
          width: 220,
          color: theme.palette.text.primary,
          borderRadius: 6,
          overflow: "clip",
          userSelect: "none",
        },
        containerActive: {
          outline: `2px solid ${theme.experimental.roles.active.outline}`,
        },
        page: {
          backgroundColor: theme.palette.background.default,
          color: theme.palette.text.primary,
        },
        header: {
          backgroundColor: theme.palette.background.paper,
          display: "flex",
          alignItems: "center",
          justifyContent: "space-between",
          padding: "6px 10px",
          marginBottom: 8,
          borderBottom: `1px solid ${theme.palette.divider}`,
        },
        headerLinks: {
          display: "flex",
          alignItems: "center",
          gap: 6,
        },
        headerLink: {
          backgroundColor: theme.palette.text.secondary,
          height: 6,
          width: 20,
          borderRadius: 3,
        },
        activeHeaderLink: {
          backgroundColor: theme.palette.text.primary,
        },
        proxy: {
          backgroundColor: theme.palette.success.light,
          height: 6,
          width: 12,
          borderRadius: 3,
        },
        user: {
          backgroundColor: theme.palette.text.primary,
          height: 8,
          width: 8,
          borderRadius: 4,
          float: "right",
        },
        body: {
          width: 120,
          margin: "auto",
        },
        title: {
          backgroundColor: theme.palette.text.primary,
          height: 8,
          width: 45,
          borderRadius: 4,
          marginBottom: 6,
        },
        table: {
          border: `1px solid ${theme.palette.divider}`,
          borderBottom: "none",
          borderTopLeftRadius: 3,
          borderTopRightRadius: 3,
          overflow: "clip",
        },
        tableHeader: {
          backgroundColor: theme.palette.background.paper,
          height: 10,
          margin: -1,
        },
        label: {
          borderTop: `1px solid ${theme.palette.divider}`,
          padding: "4px 12px",
          fontSize: 14,
        },
        workspace: {
          borderTop: `1px solid ${theme.palette.divider}`,
          height: 15,

          "&::after": {
            content: '""',
            display: "block",
            marginTop: 4,
            marginLeft: 4,
            backgroundColor: theme.palette.text.disabled,
            height: 6,
            width: 30,
            borderRadius: 3,
          },
        },
      }) satisfies Record<string, Interpolation<never>>,
    [theme],
  );

  return (
    <div
      css={[styles.container, active && styles.containerActive]}
      className={className}
    >
      <div css={styles.page}>
        <div css={styles.header}>
          <div css={styles.headerLinks}>
            <div css={[styles.headerLink, styles.activeHeaderLink]}></div>
            <div css={styles.headerLink}></div>
            <div css={styles.headerLink}></div>
          </div>
          <div css={styles.headerLinks}>
            <div css={styles.proxy}></div>
            <div css={styles.user}></div>
          </div>
        </div>
        <div css={styles.body}>
          <div css={styles.title}></div>
          <div css={styles.table}>
            <div css={styles.tableHeader}></div>
            <div css={styles.workspace}></div>
            <div css={styles.workspace}></div>
            <div css={styles.workspace}></div>
            <div css={styles.workspace}></div>
          </div>
        </div>
      </div>
      <div css={styles.label}>{displayName}</div>
    </div>
  );
};
