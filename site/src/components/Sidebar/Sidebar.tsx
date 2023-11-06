import Box, { BoxProps } from "@mui/material/Box";
import { styled } from "@mui/styles";
import { colors } from "theme/colors";

export const Sidebar = styled((props: BoxProps) => (
  <Box {...props} component="nav" />
))(({ theme }) => ({
  width: 256,
  flexShrink: 0,
  borderRight: `1px solid ${theme.palette.divider}`,
  height: "100%",
  overflowY: "auto",
}));

export const SidebarItem = styled(
  ({ active, ...props }: BoxProps & { active?: boolean }) => (
    <Box component="button" {...props} />
  ),
)(({ theme, active }) => ({
  background: active ? colors.gray[13] : "none",
  border: "none",
  fontSize: 14,
  width: "100%",
  textAlign: "left",
  padding: "0 24px",
  cursor: "pointer",
  pointerEvents: active ? "none" : "auto",
  color: active ? theme.palette.text.primary : theme.palette.text.secondary,
  "&:hover": {
    background: theme.palette.action.hover,
    color: theme.palette.text.primary,
  },
  paddingTop: 10,
  paddingBottom: 10,
}));

export const SidebarCaption = styled(Box)(({ theme }) => ({
  fontSize: 10,
  textTransform: "uppercase",
  fontWeight: 500,
  color: theme.palette.text.secondary,
  padding: "12px 24px",
  letterSpacing: "0.5px",
}));
