import TextField from "@mui/material/TextField";
import InputAdornment from "@mui/material/InputAdornment";
import Tooltip from "@mui/material/Tooltip";
import IconButton from "@mui/material/IconButton";
import Box from "@mui/material/Box";
import Link from "@mui/material/Link";
import SearchIcon from "@mui/icons-material/SearchOutlined";
import ClearIcon from "@mui/icons-material/CloseOutlined";
import { useTheme } from "@emotion/react";
import { type FC, type ReactNode, useMemo, useState } from "react";
import { Helmet } from "react-helmet-async";
import uFuzzy from "ufuzzy";
import { CopyableValue } from "components/CopyableValue/CopyableValue";
import { EmptyState } from "components/EmptyState/EmptyState";
import { Margins } from "components/Margins/Margins";
import {
  PageHeader,
  PageHeaderSubtitle,
  PageHeaderTitle,
} from "components/PageHeader/PageHeader";
import { Stack } from "components/Stack/Stack";
import icons from "theme/icons.json";
import { pageTitle } from "utils/page";

const iconsWithoutSuffix = icons.map((icon) => icon.split(".")[0]);
const fuzzyFinder = new uFuzzy({
  intraMode: 1,
  intraIns: 1,
  intraSub: 1,
  intraTrn: 1,
  intraDel: 1,
});

export const IconsPage: FC = () => {
  const theme = useTheme();
  const [searchInputText, setSearchInputText] = useState("");
  const searchText = searchInputText.trim();

  const searchedIcons = useMemo(() => {
    if (!searchText) {
      return icons.map((icon) => ({ url: `/icon/${icon}`, description: icon }));
    }

    const [map, info, sorted] = fuzzyFinder.search(
      iconsWithoutSuffix,
      searchText,
    );

    // We hit an invalid state somehow
    if (!map || !info || !sorted) {
      return [];
    }

    return sorted.map((i) => {
      const iconName = icons[info.idx[i]];
      const ranges = info.ranges[i];

      const nodes: ReactNode[] = [];
      let cursor = 0;
      for (let j = 0; j < ranges.length; j += 2) {
        nodes.push(iconName.slice(cursor, ranges[j]));
        nodes.push(
          <mark key={j + 1}>{iconName.slice(ranges[j], ranges[j + 1])}</mark>,
        );
        cursor = ranges[j + 1];
      }
      nodes.push(iconName.slice(cursor));
      return { url: `/icon/${iconName}`, description: nodes };
    });
  }, [searchText]);

  return (
    <>
      <Helmet>
        <title>{pageTitle("Icons")}</title>
      </Helmet>
      <Margins>
        <PageHeader
          actions={
            <Tooltip
              placement="bottom-end"
              title={
                <Box
                  css={{
                    padding: theme.spacing(1),
                    fontSize: 13,
                    lineHeight: 1.5,
                  }}
                >
                  You can suggest a new icon by submitting a Pull Request to our
                  public GitHub repository. Just keep in mind that it should be
                  relevant to many Coder users, and redistributable under a
                  permissive license.
                </Box>
              }
            >
              <Link href="https://github.com/coder/coder/tree/main/site/static/icon">
                Suggest an icon
              </Link>
            </Tooltip>
          }
        >
          <PageHeaderTitle>Icons</PageHeaderTitle>
          <PageHeaderSubtitle>
            All of the icons included with Coder
          </PageHeaderSubtitle>
        </PageHeader>
        <TextField
          size="small"
          InputProps={{
            "aria-label": "Filter",
            name: "query",
            placeholder: "Search…",
            value: searchInputText,
            onChange: (event) => setSearchInputText(event.target.value),
            sx: {
              borderRadius: "6px",
              marginLeft: "-1px",
              "& input::placeholder": {
                color: theme.palette.text.secondary,
              },
              "& .MuiInputAdornment-root": {
                marginLeft: 0,
              },
            },
            startAdornment: (
              <InputAdornment position="start">
                <SearchIcon
                  sx={{
                    fontSize: 14,
                    color: theme.palette.text.secondary,
                  }}
                />
              </InputAdornment>
            ),
            endAdornment: searchInputText && (
              <InputAdornment position="end">
                <Tooltip title="Clear filter">
                  <IconButton
                    size="small"
                    onClick={() => setSearchInputText("")}
                  >
                    <ClearIcon sx={{ fontSize: 14 }} />
                  </IconButton>
                </Tooltip>
              </InputAdornment>
            ),
          }}
        />

        <Stack
          direction="row"
          wrap="wrap"
          spacing={1}
          justifyContent="center"
          css={(theme) => ({ marginTop: theme.spacing(4) })}
        >
          {searchedIcons.length === 0 && (
            <EmptyState message="No results matched your search" />
          )}
          {searchedIcons.map((icon) => (
            <CopyableValue key={icon.url} value={icon.url} placement="bottom">
              <Stack alignItems="center" css={{ margin: theme.spacing(1.5) }}>
                <img
                  alt={icon.url}
                  src={icon.url}
                  css={{
                    width: 60,
                    height: 60,
                    objectFit: "contain",
                    pointerEvents: "none",
                    padding: theme.spacing(1.5),
                  }}
                />
                <figcaption
                  css={{
                    width: 88,
                    height: 48,
                    fontSize: 13,
                    textOverflow: "ellipsis",
                    textAlign: "center",
                    overflow: "hidden",
                  }}
                >
                  {icon.description}
                </figcaption>
              </Stack>
            </CopyableValue>
          ))}
        </Stack>
      </Margins>
    </>
  );
};

export default IconsPage;
