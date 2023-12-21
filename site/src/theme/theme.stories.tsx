import type { Meta, StoryObj } from "@storybook/react";
import { type FC } from "react";
import { ThemeOverride } from "contexts/ThemeProvider";
import theme from "theme";
import { InteractiveThemeRole } from "./experimental";
import { TestButton } from "./testComponents/Button/Button";
import { Callout } from "./testComponents/Callout/Callout";
import Button from "@mui/material/Button";

const meta: Meta<typeof ThemeOverride> = {
  title: "design/Theme",
  component: ThemeOverride,
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

const ButtonTasteTest: FC<{ role: InteractiveThemeRole }> = ({ role }) => {
  return (
    <div css={{ display: "flex", gap: 16 }}>
      <TestButton type={role} />
      <TestButton type={role} variant="static" />
      <TestButton type={role} variant="hover" />
      <TestButton type={role} variant="disabled" />
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
