import type { Meta, StoryObj } from "@storybook/react";
import { ThemeProvider as MuiThemeProvider } from "@mui/material/styles";
import {
  ThemeProvider as EmotionThemeProvider,
  useTheme,
} from "@emotion/react";
import { type FC, type ReactNode } from "react";
import theme, { type Theme } from "theme";
import { InteractiveThemeRole } from "./experimental";
import { Callout } from "components/Callout/Callout";
import Button from "@mui/material/Button";

interface ThemeTestingViewProps {
  theme: Theme;
  children?: ReactNode;
}

const ThemeTestingView: FC<ThemeTestingViewProps> = ({ theme, children }) => {
  return (
    <MuiThemeProvider theme={theme}>
      <EmotionThemeProvider theme={theme}>{children}</EmotionThemeProvider>
    </MuiThemeProvider>
  );
};

const meta: Meta<typeof ThemeTestingView> = {
  title: "design/Theme",
  component: ThemeTestingView,
  args: {
    theme: theme.dark,
  },
};

const ExperimentalExample: FC = () => {
  return (
    <div css={{ display: "flex", flexDirection: "column", gap: 48 }}>
      <div css={{ display: "flex", flexDirection: "column", gap: 24 }}>
        <ButtonTasteTest role="danger" />
        <ButtonTasteTest role="success" />
        <ButtonTasteTest role="active" />
      </div>
      <div css={{ display: "flex", flexDirection: "column", gap: 24 }}>
        <Callout type="danger">Hi! This is a danger callout</Callout>
        <Callout type="error">Hi! This is a error callout</Callout>
        <Callout type="warning">Hi! This is a warning callout</Callout>
        <Callout type="notice">Hi! This is a notice callout</Callout>
        <Callout type="info">Hi! This is a info callout</Callout>
        <Callout type="success">Hi! This is a success callout</Callout>
        <Callout type="active">Hi! This is a active callout</Callout>
      </div>
    </div>
  );
};

const baseButton = {
  borderRadius: 8,
  fontSize: 14,
  padding: "6px 12px",
};

const ButtonTasteTest: FC<{ role: InteractiveThemeRole }> = ({ role }) => {
  const theme = useTheme();
  const themeRole = theme.experimental.roles[role];

  return (
    <div css={{ display: "flex", gap: 16 }}>
      <button
        css={{
          marginInlineEnd: 16,

          ...baseButton,
          fontWeight: 500,
          background: themeRole.background,
          color: themeRole.text,
          border: `1px solid ${themeRole.outline}`,

          transition:
            "background 200ms ease, border 200ms ease, color 200ms ease, filter 200ms ease",

          "&:hover": {
            filter: "brightness(95%)",
            background: themeRole.hover.background,
            color: themeRole.hover.text,
            border: `1px solid ${themeRole.hover.outline}`,
          },
        }}
      >
        Do the thing
      </button>
      <button
        css={{
          ...baseButton,
          background: themeRole.background,
          color: themeRole.text,
          border: `1px solid ${themeRole.outline}`,
        }}
      >
        Do the thing
      </button>
      <button
        css={{
          ...baseButton,
          background: themeRole.hover.background,
          color: themeRole.hover.text,
          border: `1px solid ${themeRole.hover.outline}`,
        }}
      >
        Do the thing
      </button>
      <button
        css={{
          ...baseButton,
          background: themeRole.disabled.background,
          color: themeRole.disabled.text,
          border: `1px solid ${themeRole.disabled.outline}`,
        }}
        disabled
      >
        Do the thing
      </button>
    </div>
  );
};

const MuiExample: FC = () => {
  return (
    <div css={{ display: "flex", gap: 16 }}>
      <Button>Hi!</Button>
      <Button variant="contained">Hi!</Button>
      <Button variant="contained" color="primary">
        Hi!
      </Button>
      <Button variant="outlined">Hi!</Button>
    </div>
  );
};

export default meta;
type Story = StoryObj<typeof ThemeTestingView>;

export const ExperimentalDark: Story = {
  name: "Experimental (Dark)",
  args: {
    children: <ExperimentalExample />,
  },
  parameters: {
    themes: {
      themeOverride: "dark",
    },
  },
};

export const ExperimentalDarkBlue: Story = {
  name: "Experimental (Dark blue)",
  args: {
    children: <ExperimentalExample />,
    theme: theme.darkBlue,
  },
  parameters: {
    themes: {
      themeOverride: "darkBlue",
    },
  },
};

export const ExperimentalLight: Story = {
  name: "Experimental (Light)",
  args: {
    children: <ExperimentalExample />,
    theme: theme.light,
  },
  parameters: {
    themes: {
      themeOverride: "light",
    },
  },
};

export const MuiDark: Story = {
  name: "MUI (Dark)",
  args: {
    children: <MuiExample />,
  },
  parameters: {
    themes: {
      themeOverride: "dark",
    },
  },
};

export const MuiDarkBlue: Story = {
  name: "MUI (Dark Blue)",
  args: {
    children: <MuiExample />,
    theme: theme.darkBlue,
  },
  parameters: {
    themes: {
      themeOverride: "darkBlue",
    },
  },
};

export const MuiLight: Story = {
  name: "MUI (Light)",
  args: {
    children: <MuiExample />,
    theme: theme.light,
  },
  parameters: {
    themes: {
      themeOverride: "light",
    },
  },
};
